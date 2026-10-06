package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3/lock"
)

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
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT revision FROM competency_map_state").Scan(&revision); err != nil || revision != 0 {
		t.Fatalf("initial state: revision=%d, err=%v", revision, err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('auth_rate_limits') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("initial schema includes counters: exists=%v, err=%v", exists, err)
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
	initial, err := fs.ReadFile(migrations, "migrations/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"00001_initial.sql": {Data: initial},
		"00002_test.sql":    {Data: []byte("-- +goose Up\nCREATE TABLE migration_probe(id integer);\nSELECT * FROM nonexistent_migration_table;\n")},
	}
	if err := migrate(ctx, pool, files); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('migration_probe') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("failed migration was not rolled back: exists=%v, err=%v", exists, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id=2").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed migration recorded: count=%d, err=%v", count, err)
	}
	files["00002_test.sql"].Data = []byte("-- +goose Up\nCREATE TABLE migration_probe(id integer);\n")
	for range 2 {
		if err := migrate(ctx, pool, files); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id=2 AND is_applied").Scan(&count); err != nil || count != 1 {
		t.Fatalf("pending migration applied: count=%d, err=%v", count, err)
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

func TestInitialSchemaAllowsPartialOutcomeProfiles(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	initial, err := fs.ReadFile(migrations, "migrations/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, fstest.MapFS{"00001_initial.sql": {Data: initial}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,created_at) VALUES ('keep-user','keep@example.test','hash',1);
 INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers) VALUES (1,1,'keep-user',1,1,1,0,'paired','[]');
 INSERT INTO competencies(id,name,revision) VALUES ('keep-c','К',1);
 INSERT INTO constituents(id,competency_id,name) VALUES ('keep-s','keep-c','С');
 INSERT INTO outcomes(id,constituent_id,name,importance) VALUES ('keep-o','keep-s','О',3);`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE outcomes SET include_in_test=true WHERE id='keep-o'"); err != nil {
		t.Fatalf("initial schema rejected partial profile: %v", err)
	}
	var included bool
	var importance int
	if err := pool.QueryRow(ctx, "SELECT include_in_test,importance FROM outcomes WHERE id='keep-o'").Scan(&included, &importance); err != nil || !included || importance != 3 {
		t.Fatalf("existing profile changed: %v %d %v", included, importance, err)
	}
	var curriculum string
	if err := pool.QueryRow(ctx, "SELECT curriculum_sections FROM constituent_curriculum_profiles WHERE constituent_id='keep-s'").Scan(&curriculum); err != nil || curriculum != "[]" {
		t.Fatalf("initial curriculum profile: profile=%q error=%v", curriculum, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE id='keep-user'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("user lost: %d %v", count, err)
	}
}
