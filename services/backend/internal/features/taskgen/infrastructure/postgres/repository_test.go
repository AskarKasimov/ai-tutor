package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	competencypostgres "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/infrastructure/postgres"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	sharedpostgres "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTaskgenRepository(t *testing.T) (*Repository, *competencypostgres.Repository, *pgxpool.Pool, context.Context, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository integration tests")
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
	schema := "taskgen_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := sharedpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,created_at) VALUES ('taskgen-test-user','taskgen-test@example.test','hash',1)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return New(pool), competencypostgres.New(pool), pool, ctx, "taskgen-test-user"
}

func taskgenMap(name string) competencymap.Map {
	return competencymap.Map{
		SourceFormat: "paired", SourceHeaders: []string{"Компетенция", "Составляющая", "Образовательный результат"},
		Competencies: []competencymap.Competency{{Key: "c", Name: "Компетенция " + name}},
		Constituents: []competencymap.Constituent{{Key: "s", CompetencyKey: "c", Name: "Составляющая " + name}},
		Outcomes:     []competencymap.Outcome{{Key: "o", ConstituentKey: "s", Name: "ОР " + name}},
	}
}

func TestPersistGeneratedTaskAndReimportMaterial(t *testing.T) {
	repository, maps, pool, ctx, userID := setupTaskgenRepository(t)
	if _, err := maps.Replace(ctx, userID, taskgenMap("один"), 10); err != nil {
		t.Fatal(err)
	}
	var outcomeID string
	if err := pool.QueryRow(ctx, `SELECT id FROM outcomes WHERE name='ОР один'`).Scan(&outcomeID); err != nil {
		t.Fatal(err)
	}
	if err := repository.ImportMaterial(ctx, "notes", []string{outcomeID}, []string{"Материал по теме ОР один"}, 12); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.Context(ctx, outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Materials) != 1 {
		t.Fatalf("retrieved material context = %#v", snapshot.Materials)
	}
	if err := repository.ImportMaterial(ctx, "notes", []string{outcomeID}, []string{"Обновлённый материал по теме ОР один"}, 13); err != nil {
		t.Fatalf("reimport same material name: %v", err)
	}
	task, err := repository.Persist(ctx, snapshot, "request-1", userID, "test-model", application.Draft{
		Question: "Вопрос", Options: []string{}, VoiceInstruction: "Объясните ответ", ReferenceAnswer: "Эталон",
	}, 14)
	if err != nil {
		t.Fatal(err)
	}
	if task.Origin != "ai_generated" || task.ID == "" {
		t.Fatalf("generated task = %#v", task)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM material_chunks WHERE material_name='notes'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("material chunk count=%d err=%v, want immutable original and replacement", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM generation_run_chunks`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("generation context chunk count=%d err=%v", count, err)
	}
}
