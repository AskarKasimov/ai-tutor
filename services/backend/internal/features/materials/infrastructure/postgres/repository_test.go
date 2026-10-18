package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	shared "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

// repositoryPool creates a migrated isolated schema with a parsed material.
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
	schema := "material_compat_test_" + hex.EncodeToString(suffix)
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

func TestMetadataAndReplacementInvalidateOnlyChangedGraphInputs(t *testing.T) {
	pool := repositoryPool(t)
	repo := New(pool)
	ctx := context.Background()
	exec(t, pool, `UPDATE material_cleaning_jobs SET status='ready',s3_report_key='old-report'`,
		`INSERT INTO material_ocr_jobs(material_id,status,lease_token,updated_at) VALUES ('m','processing','old-ocr',1)`)
	var before string
	if err := pool.QueryRow(ctx, `SELECT fingerprint FROM course_graph_inputs WHERE subject_id='s'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	week, kind := int32(3), material.Kind("lecture")
	updated, err := repo.UpdateMetadata(ctx, "s", "m", &week, &kind)
	if err != nil || updated.UpdatedAt != 1 || updated.Cleaning == nil || updated.Cleaning.Status != "ready" {
		t.Fatalf("metadata: %+v %v", updated, err)
	}
	var after string
	if err = pool.QueryRow(ctx, `SELECT fingerprint FROM course_graph_inputs WHERE subject_id='s'`).Scan(&after); err != nil || before == after {
		t.Fatalf("metadata did not change fingerprint: %s %s %v", before, after, err)
	}
	before = after
	_, err = repo.UpdateMetadata(ctx, "s", "m", &week, &kind)
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT fingerprint FROM course_graph_inputs WHERE subject_id='s'`).Scan(&after); err != nil || before != after {
		t.Fatalf("same metadata changed fingerprint: %s %s %v", before, after, err)
	}
	updated.Filename = "replacement.pdf"
	updated.S3RawKey = "new-raw"
	updated.S3ParsedKey = "new-parsed"
	updated.UpdatedAt = 2
	replaced, err := repo.ReplaceMaterial(ctx, "s", "m", updated)
	if err != nil || replaced.Status != material.StatusPending || replaced.Processing == nil || *replaced.Week != 3 {
		t.Fatalf("replace: %+v %v", replaced, err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM course_graph_inputs WHERE subject_id='s'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("replacement still ready: %d %v", count, err)
	}
	accepted, err := repo.FinishOCR(ctx, application.OCRJob{MaterialID: "m", Token: "old-ocr"}, "old-ocr-key", "old-parsed", json.RawMessage(`{}`), "stale", nil, time.Unix(3, 0))
	if err != nil || accepted {
		t.Fatalf("accepted obsolete OCR: %v %v", accepted, err)
	}
	accepted, err = repo.FinishCleaning(ctx, material.CleaningJob{MaterialID: "m", Token: "old-cleaning"}, application.CleaningResult{Markdown: "stale"}, "stale-text", "stale-report", time.Unix(3, 0))
	if err != nil || accepted {
		t.Fatalf("accepted obsolete cleaning: %v %v", accepted, err)
	}
	job, err := repo.ClaimOCR(ctx, "new-ocr", time.Unix(4, 0))
	if err != nil || job.MaterialID != "m" || job.Attempts != 1 {
		t.Fatalf("new queue: %+v %v", job, err)
	}
	accepted, err = repo.FinishOCR(ctx, job, "new-ocr-key", "new-parsed", json.RawMessage(`{}`), "new", nil, time.Unix(5, 0))
	if err != nil || !accepted {
		t.Fatalf("finish new OCR: %v %v", accepted, err)
	}
	cleaning, err := repo.ClaimCleaning(ctx, "new-cleaning", time.Unix(6, 0))
	if err != nil || cleaning.MaterialID != "m" {
		t.Fatalf("cleaning: %+v %v", cleaning, err)
	}
	accepted, err = repo.FinishCleaning(ctx, cleaning, application.CleaningResult{Markdown: "new"}, "new-cleaned", "new-report", time.Unix(7, 0))
	if err != nil || !accepted {
		t.Fatalf("finish cleaning: %v %v", accepted, err)
	}
	if err = pool.QueryRow(ctx, `SELECT fingerprint FROM course_graph_inputs WHERE subject_id='s'`).Scan(&after); err != nil || before == after {
		t.Fatalf("replacement fingerprint unchanged: %s %s %v", before, after, err)
	}
}

// Parallel workers claim at once: each pending job goes to exactly one of
// them, and the claims beyond the queue get nothing.
func TestConcurrentClaimsTakeEachJobOnce(t *testing.T) {
	pool := repositoryPool(t)
	for _, id := range []string{"a", "b", "c"} {
		exec(t, pool,
			`INSERT INTO subject_materials(id, subject_id, filename, content_type, s3_raw_key, s3_parsed_key, status, created_at, updated_at, week, kind)
			 VALUES ('`+id+`', 's', '`+id+`.md', 'text/markdown', 'raw', 'parsed', 'parsed', 1, 1, 1, 'lecture')`,
			`INSERT INTO material_ocr_jobs(material_id, status, updated_at) VALUES ('`+id+`', 'pending', 1)`,
			`INSERT INTO material_cleaning_jobs(material_id, status, updated_at) VALUES ('`+id+`', 'pending', 1)`)
	}
	repo := New(pool)
	claim := func(name string, take func(i int) (string, error), jobs int) {
		t.Helper()
		const workers = 8
		ids := make(chan string, workers)
		errs := make(chan error, workers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				id, err := take(i)
				errs <- err
				ids <- id
			}()
		}
		close(start)
		wg.Wait()
		close(ids)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("%s claim: %v", name, err)
			}
		}
		seen := map[string]int{}
		for id := range ids {
			if id != "" {
				seen[id]++
			}
		}
		if len(seen) != jobs {
			t.Fatalf("%s: claimed %v, want %d distinct jobs", name, seen, jobs)
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("%s: job %s claimed %d times", name, id, n)
			}
		}
	}
	now := time.Unix(10, 0)
	claim("OCR", func(i int) (string, error) {
		job, err := repo.ClaimOCR(context.Background(), fmt.Sprintf("ocr-%d", i), now)
		return job.MaterialID, err
	}, 3)
	// m from the fixture has a cleaning job too.
	claim("cleaning", func(i int) (string, error) {
		job, err := repo.ClaimCleaning(context.Background(), fmt.Sprintf("clean-%d", i), now)
		return job.MaterialID, err
	}, 4)
}
