package labels

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	sharedpostgres "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresSaveGraphLabelsRollsBackTheWholeRequest(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for graph label transaction tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "label_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := sharedpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	valid := db.LearningSaveGraphLabelParams{CacheKey: "first", DisplayName: "Нейронные сети"}
	invalid := db.LearningSaveGraphLabelParams{CacheKey: "second", DisplayName: strings.Repeat("я", 81)}
	if err := store.SaveGraphLabels(ctx, []db.LearningSaveGraphLabelParams{valid, invalid}); err == nil {
		t.Fatal("expected database constraint failure")
	}
	rows, err := store.LearningReadGraphLabels(ctx, []string{"first", "second"})
	if err != nil || len(rows) != 0 {
		t.Fatalf("partially committed cache: %+v, %v", rows, err)
	}
	invalid.DisplayName = "Градиентный спуск"
	if err := store.SaveGraphLabels(ctx, []db.LearningSaveGraphLabelParams{valid, invalid}); err != nil {
		t.Fatal(err)
	}
	rows, err = store.LearningReadGraphLabels(ctx, []string{"first", "second"})
	if err != nil || len(rows) != 2 {
		t.Fatalf("retry did not persist the complete cache: %+v, %v", rows, err)
	}
}
