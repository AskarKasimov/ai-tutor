package graph

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	shared "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

func storedPool(t *testing.T) *pgxpool.Pool {
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
	_, _ = rand.Read(suffix)
	schema := "graph_test_" + hex.EncodeToString(suffix)
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
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	if err := shared.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO subjects(id, name, created_at) VALUES ('s', 'Курс', 1)`,
		`INSERT INTO subject_materials(id, subject_id, filename, content_type, s3_raw_key, s3_parsed_key, status, created_at, updated_at)
		 VALUES ('m', 's', 'week1.md', 'text/markdown', 'raw', 'parsed', 'parsed', 1, 1)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func publish(t *testing.T, pool *pgxpool.Pool, fingerprint, title string) {
	t.Helper()
	graph := `{"version":"course-graph-v1","topics":[{"id":"t1","title":"` + title + `","text":"Текст"},{"id":"t2","title":"Основа","text":"Текст"}],` +
		`"prerequisites":[{"source":"t2","target":"t1"}],"hierarchy":[]}`
	if _, err := pool.Exec(context.Background(), `INSERT INTO course_graphs(subject_id, fingerprint, graph, updated_at) VALUES ('s', $1, $2, 1)
		ON CONFLICT(subject_id) DO UPDATE SET fingerprint=excluded.fingerprint, graph=excluded.graph, updated_at=2`, fingerprint, graph); err != nil {
		t.Fatal(err)
	}
}

func TestStoredGraphVersionChangesOnRebuild(t *testing.T) {
	pool := storedPool(t)
	ctx := context.Background()
	publish(t, pool, "one", "Производная")
	first, ok, err := NewStored(pool).Current(ctx, "s")
	if err != nil || !ok {
		t.Fatalf("first graph: %v %v", ok, err)
	}
	if len(first.Nodes) != 2 || first.Nodes[0].Prerequisites[0] != "t2" {
		t.Fatalf("nodes = %+v", first.Nodes)
	}
	again, _, _ := NewStored(pool).Current(ctx, "s")
	publish(t, pool, "two", "Интеграл")
	second, _, err := NewStored(pool).Current(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != again.Version {
		t.Fatalf("same graph read twice has versions %s and %s", first.Version, again.Version)
	}
	if first.Version == second.Version || first.Version == "course-graph-v1" {
		t.Fatalf("rebuilt graph keeps version %s", second.Version)
	}
}

func TestStoredGraphReturnsDatabaseErrors(t *testing.T) {
	pool := storedPool(t)
	publish(t, pool, "one", "Производная")
	pool.Close()
	if _, ok, err := NewStored(pool).Current(context.Background(), "s"); err == nil || ok {
		t.Fatalf("closed pool: ok=%v err=%v, want the database error", ok, err)
	}
}

func TestStoredGraphOfCourseWithoutMaterialsIsNotServed(t *testing.T) {
	pool := storedPool(t)
	ctx := context.Background()
	publish(t, pool, "one", "Производная")
	if _, err := pool.Exec(ctx, `DELETE FROM subject_materials`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := NewStored(pool).Current(ctx, "s"); ok || err != nil {
		t.Fatalf("course without materials: ok=%v err=%v", ok, err)
	}
	// Historical map graphs remain stored, but cannot back a new exam.
	if _, err := pool.Exec(ctx, `UPDATE course_graphs SET source='competency_map'`); err != nil {
		t.Fatal(err)
	}
	if g, ok, err := NewStored(pool).Current(ctx, "s"); err != nil || ok {
		t.Fatalf("historical map graph without materials: ok=%v err=%v nodes=%d", ok, err, len(g.Nodes))
	}
}
