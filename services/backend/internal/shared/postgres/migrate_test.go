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

func TestSubjectsMigrationBackfillsCurrentMapAndVariants(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	files := legacySubjectMigrationFiles(t)
	if err := migrate(ctx, pool, files); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `
INSERT INTO users(id,email,password_hash,created_at) VALUES ('subject-user','subject@example.test','hash',1);
INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers)
VALUES (7,1,'subject-user',1,1,1,1,'paired','[]');
INSERT INTO competencies(id,name,revision) VALUES ('subject-c','Компетенция ML',7);
INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at)
VALUES ('subject-v','subject-user','subject-request',7,'test',1,'[]',1);
INSERT INTO variant_tasks(id,variant_id,source_task_id_snapshot,competency_position,slot,role,task_snapshot,profile_snapshot)
VALUES ('subject-vt','subject-v','historical-task',1,0,'main','{"question":"Снимок"}','{}');
UPDATE competency_map_state SET revision=7 WHERE singleton=true;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	const mlID = "subject:intro-to-ml"
	var subjectName string
	var activeRevision *int64
	if err := pool.QueryRow(ctx, `SELECT name, active_revision FROM subjects WHERE id=$1`, mlID).Scan(&subjectName, &activeRevision); err != nil {
		t.Fatal(err)
	}
	if subjectName != "Введение в ML" || activeRevision == nil || *activeRevision != 7 {
		t.Fatalf("ML subject backfill: name=%q active_revision=%v", subjectName, activeRevision)
	}
	var importSubject string
	if err := pool.QueryRow(ctx, `SELECT subject_id FROM competency_map_imports WHERE revision=7`).Scan(&importSubject); err != nil || importSubject != mlID {
		t.Fatalf("import subject=%q err=%v", importSubject, err)
	}
	var variantSubject, variantName, taskSnapshot string
	if err := pool.QueryRow(ctx, `SELECT subject_id, subject_name_snapshot, task_snapshot->>'question' FROM variants v JOIN variant_tasks vt ON vt.variant_id=v.id WHERE v.id='subject-v'`).Scan(&variantSubject, &variantName, &taskSnapshot); err != nil {
		t.Fatal(err)
	}
	if variantSubject != mlID || variantName != "Введение в ML" || taskSnapshot != "Снимок" {
		t.Fatalf("variant backfill: subject=%q name=%q task snapshot=%q", variantSubject, variantName, taskSnapshot)
	}
	var subjectIDDefault, subjectNameDefault *string
	if err := pool.QueryRow(ctx, `SELECT
  (SELECT column_default FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='variants' AND column_name='subject_id'),
  (SELECT column_default FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='variants' AND column_name='subject_name_snapshot')`).Scan(&subjectIDDefault, &subjectNameDefault); err != nil {
		t.Fatal(err)
	}
	if subjectIDDefault != nil || subjectNameDefault != nil {
		t.Fatalf("variant subject defaults remain: subject_id=%v subject_name_snapshot=%v", subjectIDDefault, subjectNameDefault)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subjects(id,name,created_at) VALUES ('subject:second','Второй предмет',2)`); err != nil {
		t.Fatalf("insert second subject: %v", err)
	}
	var subjects int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM subjects WHERE id IN ($1,'subject:second')`, mlID).Scan(&subjects); err != nil || subjects != 2 {
		t.Fatalf("subjects=%d err=%v", subjects, err)
	}
	var mlRevisionAfterInsert *int64
	var mlNameAfterInsert string
	if err := pool.QueryRow(ctx, `SELECT name, active_revision FROM subjects WHERE id=$1`, mlID).Scan(&mlNameAfterInsert, &mlRevisionAfterInsert); err != nil || mlNameAfterInsert != "Введение в ML" || mlRevisionAfterInsert == nil || *mlRevisionAfterInsert != 7 {
		t.Fatalf("ML subject changed after adding second subject: name=%q revision=%v err=%v", mlNameAfterInsert, mlRevisionAfterInsert, err)
	}
	var globalRevision int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM competency_map_state WHERE singleton=true`).Scan(&globalRevision); err != nil || globalRevision != 7 {
		t.Fatalf("global revision=%d err=%v", globalRevision, err)
	}
}

func legacySubjectMigrationFiles(t *testing.T) fs.FS {
	t.Helper()
	files := fstest.MapFS{}
	for _, name := range []string{"00001_initial.sql", "00002_variants.sql", "00003_task_audio.sql"} {
		data, err := fs.ReadFile(migrations, "migrations/"+name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	return files
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
		"00009_test.sql":    {Data: []byte("-- +goose Up\nCREATE TABLE migration_probe(id integer);\nSELECT * FROM nonexistent_migration_table;\n")},
	}
	if err := migrate(ctx, pool, files); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('migration_probe') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("failed migration was not rolled back: exists=%v, err=%v", exists, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id=9").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed migration recorded: count=%d, err=%v", count, err)
	}
	files["00009_test.sql"].Data = []byte("-- +goose Up\nCREATE TABLE migration_probe(id integer);\n")
	for range 2 {
		if err := migrate(ctx, pool, files); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id=9 AND is_applied").Scan(&count); err != nil || count != 1 {
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

func legacyMigrationFiles(t *testing.T) fs.FS {
	t.Helper()
	initial, err := fs.ReadFile(migrations, "migrations/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	variants, err := fs.ReadFile(migrations, "migrations/00002_variants.sql")
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"00001_initial.sql":  {Data: initial},
		"00002_variants.sql": {Data: variants},
	}
}

func seedLegacyAudioRows(t *testing.T, pool *pgxpool.Pool, conflicting bool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO users(id,email,password_hash,created_at) VALUES ('audio-user','audio@example.test','hash',1);
INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers)
VALUES (1,1,'audio-user',1,1,1,1,'paired','[]');
INSERT INTO competencies(id,name,revision) VALUES ('audio-c','Компетенция',1);
INSERT INTO constituents(id,competency_id,name) VALUES ('audio-s','audio-c','Составляющая');
INSERT INTO outcomes(id,constituent_id,name) VALUES ('audio-o','audio-s','Результат');
INSERT INTO tasks(id,outcome_id,question,voice_instruction,created_at)
VALUES ('task-current','audio-o','Вопрос','Прочитайте текущее задание',1);
INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at)
VALUES ('audio-v','audio-user','audio-request',1,'test',1,'[]',1);
INSERT INTO variant_tasks(id,variant_id,live_task_id,source_task_id_snapshot,competency_position,slot,role,task_snapshot,profile_snapshot)
VALUES
 ('audio-vt-current','audio-v','task-current','task-current',1,0,'main',
  '{"id":"task-current","voice_instruction":"Прочитайте текущее задание"}','{}'),
 ('audio-vt-old','audio-v',NULL,'task-old',1,1,'basic',
  '{"id":"task-old","voice_instruction":"Прочитайте сохранённое задание"}','{}');`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE competency_map_state SET revision=1 WHERE singleton=true`); err != nil {
		t.Fatal(err)
	}
	if conflicting {
		if _, err := pool.Exec(ctx, `INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at) VALUES ('audio-v2','audio-user','audio-request-2',1,'test',1,'[]',2)`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO variant_tasks(id,variant_id,source_task_id_snapshot,competency_position,slot,role,task_snapshot,profile_snapshot)
VALUES ('audio-vt-conflict','audio-v2','task-current',2,0,'main','{"id":"task-current","voice_instruction":"Другая инструкция"}','{}')`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTaskAudioMigrationBackfillsLiveAndSnapshotOnlyTasks(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := migrate(ctx, pool, legacyMigrationFiles(t)); err != nil {
		t.Fatal(err)
	}
	seedLegacyAudioRows(t, pool, false)
	files := legacyMigrationFiles(t).(fstest.MapFS)
	audioSQL, err := fs.ReadFile(migrations, "migrations/00003_task_audio.sql")
	if err != nil {
		t.Fatal(err)
	}
	files["00003_task_audio.sql"] = &fstest.MapFile{Data: audioSQL}
	if err := migrate(ctx, pool, files); err != nil {
		t.Fatal(err)
	}
	for id, wantStatus := range map[string]string{"taskaudio_task-current": "pending", "taskaudio_task-old": "cancelled"} {
		var status string
		var instruction string
		var audioURL *string
		if err := pool.QueryRow(ctx, `SELECT status,instruction,audio_url FROM audio_assets WHERE id=$1`, id).Scan(&status, &instruction, &audioURL); err != nil {
			t.Fatal(err)
		}
		if status != wantStatus || audioURL != nil {
			t.Fatalf("backfilled asset %s: status=%s want=%s url=%v", id, status, wantStatus, audioURL)
		}
	}
	var taskAsset, snapshotAsset *string
	if err := pool.QueryRow(ctx, `SELECT audio_asset_id FROM tasks WHERE id='task-current'`).Scan(&taskAsset); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT audio_asset_id FROM variant_tasks WHERE id='audio-vt-old'`).Scan(&snapshotAsset); err != nil {
		t.Fatal(err)
	}
	if taskAsset == nil || *taskAsset != "taskaudio_task-current" || snapshotAsset == nil || *snapshotAsset != "taskaudio_task-old" {
		t.Fatalf("backfilled links: task=%v snapshot=%v", taskAsset, snapshotAsset)
	}
}

func TestTaskAudioMigrationRejectsConflictingInstructions(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := migrate(ctx, pool, legacyMigrationFiles(t)); err != nil {
		t.Fatal(err)
	}
	seedLegacyAudioRows(t, pool, true)
	if err := Migrate(ctx, pool); err == nil {
		t.Fatal("migration accepted conflicting instructions for a source task")
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('audio_assets') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("failed migration was not rolled back: exists=%v err=%v", exists, err)
	}
}
