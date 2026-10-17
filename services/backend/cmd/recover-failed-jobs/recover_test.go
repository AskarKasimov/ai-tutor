package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	shared "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

// recoveryPool is a migrated schema with two courses: material a1 of course A
// and b1 of course B, each with failed OCR, cleaning and graph jobs, and a
// stalled OCR job of a2 leased with token "t-a2".
func recoveryPool(t *testing.T) *pgxpool.Pool {
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
	schema := "recovery_test_" + hex.EncodeToString(suffix)
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
		`INSERT INTO subjects(id, name, created_at) VALUES ('A', 'Курс А', 1), ('B', 'Курс Б', 1)`,
		`INSERT INTO subject_materials(id, subject_id, filename, content_type, s3_raw_key, s3_parsed_key, status, error_message, created_at, updated_at)
		 VALUES ('a1','A','a1.pdf','application/pdf','r','p','failed','OCR failed',1,1),
		        ('a2','A','a2.pdf','application/pdf','r','p','pending',NULL,1,1),
		        ('b1','B','b1.pdf','application/pdf','r','p','failed','OCR failed',1,1)`,
		`DELETE FROM material_ocr_jobs`,
		`INSERT INTO material_ocr_jobs(material_id, status, attempts, error_message, updated_at)
		 VALUES ('a1','failed',3,'boom',1), ('b1','failed',3,'boom',2)`,
		`INSERT INTO material_ocr_jobs(material_id, status, attempts, lease_until, lease_token, updated_at)
		 VALUES ('a2','processing',1,10,'t-a2',3)`,
		`INSERT INTO material_cleaning_jobs(material_id, status, attempts, updated_at) VALUES ('a1','failed',3,1), ('b1','failed',3,2)`,
		`INSERT INTO course_graph_jobs(subject_id, fingerprint, status, attempts, updated_at) VALUES ('A','fa','failed',3,1), ('B','fb','failed',3,2)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func status(t *testing.T, pool *pgxpool.Pool, query, id string) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(), query, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

const (
	ocrStatus      = `SELECT status FROM material_ocr_jobs WHERE material_id=$1`
	cleaningStatus = `SELECT status FROM material_cleaning_jobs WHERE material_id=$1`
	graphStatus    = `SELECT status FROM course_graph_jobs WHERE subject_id=$1`
	materialStatus = `SELECT status FROM subject_materials WHERE id=$1`
)

func TestRecoverDryRunChangesNothing(t *testing.T) {
	pool := recoveryPool(t)
	var out bytes.Buffer
	sum, err := RecoverJobs(context.Background(), pool, Options{DryRun: true, Now: time.Unix(100, 0)}, &out)
	if err != nil {
		t.Fatal(err)
	}
	// Two failed OCR, two cleaning and two graph jobs; the stalled one needs --stalled.
	if sum.CandidatesFound != 6 || sum.Recovered != 6 || sum.Skipped != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if got := status(t, pool, ocrStatus, "a1"); got != "failed" {
		t.Fatalf("dry run changed OCR job: %s", got)
	}
	if !strings.Contains(out.String(), `"action":"dry_run_recover"`) {
		t.Fatalf("audit log = %s", out.String())
	}
}

func TestRecoverResetsFailedJobsAndMaterial(t *testing.T) {
	pool := recoveryPool(t)
	sum, err := RecoverJobs(context.Background(), pool, Options{Now: time.Unix(100, 0)}, nil)
	if err != nil || sum.Recovered != 6 {
		t.Fatalf("summary = %+v, %v", sum, err)
	}
	for _, check := range [][2]string{{ocrStatus, "a1"}, {cleaningStatus, "b1"}, {graphStatus, "A"}, {materialStatus, "a1"}} {
		if got := status(t, pool, check[0], check[1]); got != "pending" {
			t.Fatalf("%s %s = %s, want pending", check[0], check[1], got)
		}
	}
}

func TestRecoverMaterialFilterKeepsOtherCourses(t *testing.T) {
	pool := recoveryPool(t)
	sum, err := RecoverJobs(context.Background(), pool, Options{MaterialID: "a1", Now: time.Unix(100, 0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// OCR and cleaning of a1 and the graph of its course A only.
	if sum.Recovered != 3 {
		t.Fatalf("summary = %+v", sum)
	}
	if got := status(t, pool, graphStatus, "A"); got != "pending" {
		t.Fatalf("graph of the material's course = %s, want pending", got)
	}
	for _, check := range [][2]string{{ocrStatus, "b1"}, {cleaningStatus, "b1"}, {graphStatus, "B"}, {materialStatus, "b1"}} {
		if got := status(t, pool, check[0], check[1]); got != "failed" {
			t.Fatalf("other course changed: %s %s = %s", check[0], check[1], got)
		}
	}
}

func TestRecoverStalledChecksToken(t *testing.T) {
	pool := recoveryPool(t)
	opts := Options{Stage: "ocr", MaterialID: "a2", Stalled: true, Token: "other", Now: time.Unix(100, 0)}
	sum, err := RecoverJobs(context.Background(), pool, opts, nil)
	if err != nil || sum.Skipped != 1 || sum.Recovered != 0 {
		t.Fatalf("wrong token: %+v, %v", sum, err)
	}
	opts.Token = "t-a2"
	if sum, err = RecoverJobs(context.Background(), pool, opts, nil); err != nil || sum.Recovered != 1 {
		t.Fatalf("matching token: %+v, %v", sum, err)
	}
	if got := status(t, pool, ocrStatus, "a2"); got != "pending" {
		t.Fatalf("stalled job = %s, want pending", got)
	}
}

func TestRecoverRejectsUnknownStage(t *testing.T) {
	if _, err := RecoverJobs(context.Background(), nil, Options{Stage: "translation"}, nil); err == nil {
		t.Fatal("unknown stage accepted")
	}
}
