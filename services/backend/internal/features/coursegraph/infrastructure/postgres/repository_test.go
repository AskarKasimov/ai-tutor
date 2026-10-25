package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	shared "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

// repositoryPool has course s with material m: OCR done, cleaning pending.
func repositoryPool(t *testing.T) *pgxpool.Pool {
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
	schema := "coursegraph_test_" + hex.EncodeToString(suffix)
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
	exec(t, pool,
		`INSERT INTO subjects(id, name, created_at) VALUES ('s', 'Курс', 1)`,
		`INSERT INTO subject_materials(id, subject_id, filename, content_type, s3_raw_key, s3_parsed_key, status, created_at, updated_at, week, kind)
		 VALUES ('m', 's', 'week1.md', 'text/markdown', 'raw', 'parsed', 'parsed', 1, 1, 1, 'lecture')`,
		`INSERT INTO material_cleaning_jobs(material_id, status, updated_at) VALUES ('m', 'pending', 1)`)
	return pool
}

func exec(t *testing.T, pool *pgxpool.Pool, queries ...string) {
	t.Helper()
	for _, q := range queries {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildStartsOnlyOnRequestOfReadyMaterials(t *testing.T) {
	pool := repositoryPool(t)
	repo := New(pool)
	ctx, now := context.Background(), time.Unix(100, 0)

	snapshot, err := repo.Read(ctx, "s")
	if err != nil || snapshot.Status != "not_requested" || snapshot.InputsReady {
		t.Fatalf("before: %+v %v", snapshot, err)
	}
	if queued, err := repo.Request(ctx, "s", now); err != nil || queued {
		t.Fatalf("request with unprocessed material: %v %v", queued, err)
	}

	exec(t, pool, `UPDATE material_cleaning_jobs SET status='ready', s3_report_key='r1'`)
	// Ready materials alone do not start a build: that is the button.
	if job, err := repo.Claim(ctx, "w1", now); err != nil || job.SubjectID != "" {
		t.Fatalf("claim without request: %+v %v", job, err)
	}
	if queued, err := repo.Request(ctx, "s", now); err != nil || !queued {
		t.Fatalf("request: %v %v", queued, err)
	}
	if queued, _ := repo.Request(ctx, "s", now); queued {
		t.Fatal("second request while the build is queued restarted it")
	}

	// A material changed after the request: the build takes the current set.
	exec(t, pool, `UPDATE material_cleaning_jobs SET s3_report_key='r2'`)
	job, err := repo.Claim(ctx, "w1", now)
	if err != nil || job.SubjectID != "s" {
		t.Fatalf("claim: %+v %v", job, err)
	}
	accepted, err := repo.Finish(ctx, job, json.RawMessage(`{"version":"course-graph-v1","topics":[]}`), now)
	if err != nil || !accepted {
		t.Fatalf("finish with current inputs: %v %v", accepted, err)
	}
	snapshot, err = repo.Read(ctx, "s")
	if err != nil || snapshot.Status != "ready" || snapshot.Stale || !snapshot.InputsReady {
		t.Fatalf("after build: %+v %v", snapshot, err)
	}
}

func TestBuildChangedDuringRunIsNotPublished(t *testing.T) {
	pool := repositoryPool(t)
	repo := New(pool)
	ctx, now := context.Background(), time.Unix(100, 0)
	exec(t, pool, `UPDATE material_cleaning_jobs SET status='ready', s3_report_key='r1'`)
	if queued, err := repo.Request(ctx, "s", now); err != nil || !queued {
		t.Fatalf("request: %v %v", queued, err)
	}
	job, err := repo.Claim(ctx, "w1", now)
	if err != nil || job.SubjectID != "s" {
		t.Fatalf("claim: %+v %v", job, err)
	}
	exec(t, pool, `UPDATE material_cleaning_jobs SET s3_report_key='r2'`, `UPDATE subject_materials SET week=2 WHERE id='m'`)
	if accepted, err := repo.Finish(ctx, job, json.RawMessage(`{}`), now); err != nil || accepted {
		t.Fatalf("finish with stale inputs: %v %v", accepted, err)
	}
	if err := repo.Release(ctx, job, now); err != nil {
		t.Fatal(err)
	}
	again, err := repo.Claim(ctx, "w2", now)
	if err != nil || again.SubjectID != "s" || again.Fingerprint == job.Fingerprint || again.Attempts != job.Attempts {
		t.Fatalf("reclaim: %+v (was %+v) %v", again, job, err)
	}
}

func TestInputsAreNotReadyUntilMaterialMetadataIsComplete(t *testing.T) {
	pool := repositoryPool(t)
	ctx := context.Background()
	exec(t, pool, `UPDATE material_cleaning_jobs SET status='ready', s3_report_key='r1'`,
		`UPDATE subject_materials SET week=NULL WHERE id='m'`)
	snapshot, err := New(pool).Read(ctx, "s")
	if err != nil || snapshot.InputsReady {
		t.Fatalf("snapshot = %+v err=%v", snapshot, err)
	}
	if queued, err := New(pool).Request(ctx, "s", time.Unix(100, 0)); err != nil || queued {
		t.Fatalf("queued incomplete material: %v err=%v", queued, err)
	}
}

func TestPublishedListsTheGraphsGeekExaminesBy(t *testing.T) {
	pool := repositoryPool(t)
	exec(t, pool,
		`INSERT INTO subjects(id, name, created_at) VALUES ('empty', 'Без материалов', 1)`,
		`INSERT INTO course_graphs(subject_id, fingerprint, graph, updated_at) VALUES ('s', 'f', '{"topics":[{"question":"Почему?"}]}', 2)`,
		`INSERT INTO course_graphs(subject_id, fingerprint, graph, updated_at) VALUES ('empty', 'f', '{"topics":[{"question":"Лишний?"}]}', 3)`)
	graphs, err := New(pool).Published(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// A graph of materials is not examined once the course has none.
	if len(graphs) != 1 {
		t.Fatalf("published graphs = %d", len(graphs))
	}
	var g struct{ Topics []struct{ Question string } }
	if err := json.Unmarshal(graphs[0], &g); err != nil || len(g.Topics) != 1 || g.Topics[0].Question != "Почему?" {
		t.Fatalf("graph = %s, %v", graphs[0], err)
	}
}

func TestAMapIsBuiltWithoutMaterialsAndReplacesTheGraph(t *testing.T) {
	pool := repositoryPool(t)
	repo := New(pool)
	ctx, now := context.Background(), time.Unix(100, 0)
	graph := json.RawMessage(`{"version":"competency-map-v1","source":"competency_map","topics":[{"id":"a","title":"ОР"}]}`)
	// The materials of s are still being cleaned: a map does not wait for them.
	if queued, err := repo.SaveMap(ctx, "s", "map.xlsx", "Карта", graph, now); err != nil || !queued {
		t.Fatalf("save map: %v %v", queued, err)
	}
	if queued, err := repo.SaveMap(ctx, "s", "other.xlsx", "Карта", graph, now); err != nil || queued {
		t.Fatalf("a second map during the build: %v %v", queued, err)
	}
	job, err := repo.Claim(ctx, "w1", now)
	if err != nil || job.SubjectID != "s" || job.Source != "competency_map" {
		t.Fatalf("claim: %+v %v", job, err)
	}
	stored, err := repo.MapGraph(ctx, "s")
	// jsonb keeps the content, not the key order.
	if err != nil || !strings.Contains(string(stored), `"version": "competency-map-v1"`) {
		t.Fatalf("map graph: %s %v", stored, err)
	}
	if accepted, err := repo.Finish(ctx, job, graph, now); err != nil || !accepted {
		t.Fatalf("finish: %v %v", accepted, err)
	}
	snapshot, err := repo.Read(ctx, "s")
	if err != nil || snapshot.GraphSource != "competency_map" || snapshot.Stale || snapshot.Map == nil ||
		snapshot.Map.Filename != "map.xlsx" || snapshot.BuildSource == nil || *snapshot.BuildSource != "competency_map" {
		t.Fatalf("snapshot = %+v %v", snapshot, err)
	}
	// Geek reads a graph of a map after the last material is gone.
	exec(t, pool, `DELETE FROM subject_materials`)
	published, err := repo.Published(ctx)
	if err != nil || len(published) != 1 {
		t.Fatalf("published: %d %v", len(published), err)
	}
}
