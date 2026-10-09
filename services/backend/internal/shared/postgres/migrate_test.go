package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
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
	var subjects, imports int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM subjects").Scan(&subjects); err != nil || subjects != 0 {
		t.Fatalf("fresh subjects=%d err=%v", subjects, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM competency_map_imports").Scan(&imports); err != nil || imports != 0 {
		t.Fatalf("fresh imports=%d err=%v", imports, err)
	}
	var versions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&versions); err != nil || versions != 5 {
		t.Fatalf("applied migrations=%d err=%v, want 5", versions, err)
	}
}

func TestMigrateDownRemovesCyclicSubjectSchema(t *testing.T) {
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
('subjects','competency_map_imports','competency_map_state','variants','audio_assets','diagnostic_sessions','training_sessions')`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("schema tables remain after rollback: %d", remaining)
	}
}

func TestSubjectRequiredWithoutDefaults(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,created_at) VALUES ('u','u@example.test','hash',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers) VALUES (1,1,'u',0,0,0,0,'paired','[]')`); err == nil {
		t.Fatal("import without subject_id succeeded")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at) VALUES ('v','u','key',0,'test',1,'[]',1)`); err == nil {
		t.Fatal("variant without subject fields succeeded")
	}
	for _, column := range []struct{ table, name string }{{"competency_map_imports", "subject_id"}, {"variants", "subject_id"}, {"variants", "subject_name_snapshot"}} {
		var def *string
		if err := pool.QueryRow(ctx, `SELECT column_default FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2`, column.table, column.name).Scan(&def); err != nil {
			t.Fatal(err)
		}
		if def != nil {
			t.Fatalf("%s.%s default = %q", column.table, column.name, *def)
		}
	}
}

func TestActiveRevisionForeignKeyAllowsReimport(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `
INSERT INTO users(id,email,password_hash,created_at) VALUES ('u','u@example.test','hash',1);
INSERT INTO subjects(id,name,created_at) VALUES ('s','Subject',1);
INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers,subject_id)
VALUES (1,1,'u',1,1,1,1,'paired','[]','s');
INSERT INTO competencies(id,name,revision) VALUES ('c','Competency',1);
INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at,subject_id,subject_name_snapshot)
VALUES ('v','u','request',1,'test',1,'[]',1,'s','Subject');
INSERT INTO variant_tasks(id,variant_id,source_task_id_snapshot,competency_position,slot,role,task_snapshot,profile_snapshot)
VALUES ('vt','v','old-task',1,0,'main','{"question":"Historical snapshot"}','{}');
UPDATE subjects SET active_revision=1 WHERE id='s';`)
	if err != nil {
		t.Fatal(err)
	}
	var fk string
	if err := pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='subjects_active_revision_fkey'`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fk, "ON DELETE SET NULL") {
		t.Fatalf("active revision FK = %q", fk)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM competency_map_imports WHERE subject_id='s'`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	var active *int64
	if err := tx.QueryRow(ctx, `SELECT active_revision FROM subjects WHERE id='s'`).Scan(&active); err != nil || active != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("active revision after import delete=%v err=%v", active, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers,subject_id) VALUES (2,2,'u',1,1,1,1,'paired','[]','s')`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE subjects SET active_revision=2 WHERE id='s'`); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var snapshot string
	if err := pool.QueryRow(ctx, `SELECT task_snapshot->>'question' FROM variant_tasks WHERE id='vt'`).Scan(&snapshot); err != nil || snapshot != "Historical snapshot" {
		t.Fatalf("snapshot=%q err=%v", snapshot, err)
	}
}

func TestInitialSchemaAllowsPartialOutcomeProfiles(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,created_at) VALUES ('u','u@example.test','hash',1);
INSERT INTO subjects(id,name,created_at) VALUES ('s','Subject',1);
INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers,subject_id) VALUES (1,1,'u',1,1,1,0,'paired','[]','s');
INSERT INTO competencies(id,name,revision) VALUES ('c','К',1);
INSERT INTO constituents(id,competency_id,name) VALUES ('const','c','С');
INSERT INTO outcomes(id,constituent_id,name,importance) VALUES ('o','const','О',3);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE outcomes SET include_in_test=true WHERE id='o'"); err != nil {
		t.Fatalf("initial schema rejected partial profile: %v", err)
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
