package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

func TestEmbeddedMigrationsInitializeWithoutDatabase(t *testing.T) {
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	// Provider initialization checks migration versions without connecting to PostgreSQL.
	db, err := sql.Open("pgx", "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithDisableGlobalRegistry(true)); err != nil {
		t.Fatalf("initialize embedded migrations: %v", err)
	}
}

func migrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "migration_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return pool
}

func assertGooseVersion(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM goose_db_version WHERE version_id=1 AND is_applied").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("initial migration recorded %d times, want 1", count)
	}
}

func TestMigrateFreshAndRepeat(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	for range 2 {
		if err := Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	assertGooseVersion(t, pool)
	var subjects int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM subjects").Scan(&subjects); err != nil || subjects != 0 {
		t.Fatalf("fresh subjects=%d err=%v", subjects, err)
	}
	var versions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("applied migrations=%d err=%v, want 2", versions, err)
	}
}

func TestMigrateDownRemovesSchema(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*pool.Config().ConnConfig)
	defer db.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithSessionLocker(locker), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("rollback all migrations: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
WHERE table_schema=current_schema() AND table_name IN
('users','auth_sessions','transcriptions','subjects','subject_materials','geek_sessions','geek_requests')`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("schema tables remain after rollback: %d", remaining)
	}
}

func TestMigrateConcurrent(t *testing.T) {
	pool := migrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() { errs <- Migrate(ctx, pool) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertGooseVersion(t, pool)
}

func TestMigratePendingRollbackAndRetry(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"99999_probe.sql": {Data: []byte("-- +goose Up\nCREATE TABLE migration_probe(id integer);\nSELECT * FROM nonexistent_migration_table;\n")}}
	if err := migrate(ctx, pool, files); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('migration_probe') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("failed migration not rolled back: exists=%v err=%v", exists, err)
	}
	files["99999_probe.sql"].Data = []byte("-- +goose Up\nCREATE TABLE migration_probe(id integer);\n")
	if err := migrate(ctx, pool, files); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateWaitsForLockAndCanRetryAfterCancellation(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatal(err)
	}
	lockedCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if err := Migrate(lockedCtx, pool); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("migration did not wait for lock: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	assertGooseVersion(t, pool)
}
