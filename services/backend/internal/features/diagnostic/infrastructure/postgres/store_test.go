package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	sharedpostgres "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupStore(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL diagnostic store tests")
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
	schema := "diagnostic_store_test_" + hex.EncodeToString(suffix)
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
	if _, err := pool.Exec(ctx, `
INSERT INTO users(id,email,password_hash,created_at) VALUES
 ('diag-owner-a','diag-a@example.test','hash',1),
 ('diag-owner-b','diag-b@example.test','hash',1);
INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at,subject_id,subject_name_snapshot)
VALUES
 ('variant-a','diag-owner-a','variant-key-a',0,'test',1,'[]',1,'subject:intro-to-ml','Введение в ML'),
 ('variant-b','diag-owner-b','variant-key-b',0,'test',1,'[]',1,'subject:intro-to-ml','Введение в ML');`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return pool, ctx
}

func TestStoreStartPersistsIdempotencyAndOwnerAcrossRepositoryInstances(t *testing.T) {
	pool, ctx := setupStore(t)
	store := New(pool)
	want := diagnostic.Session{
		ID: "diagnostic-a", OwnerID: "diag-owner-a", VariantID: "variant-a", Status: diagnostic.StatusActive,
		Variant: diagnostic.VariantSnapshot{ID: "variant-a", MapRevision: 0, AlgorithmVersion: "test", IncludedCompetencyCount: 1,
			Competencies: []diagnostic.Competency{{ID: "competency-a", Name: "Algebra", Position: 1, Tasks: []diagnostic.TaskSnapshot{{
				ID: "variant-task-a", SourceTaskID: "task-a", Role: "main", CompetencyID: "competency-a", CompetencyName: "Algebra",
				OutcomeID: "outcome-a", OutcomeName: "Linear equations", Question: "Solve x + 1 = 2", Options: []string{"1", "2"},
				VoiceInstruction: "Explain the answer", ReferenceAnswer: "1", IncludeInTest: true,
			}}}},
		},
		AcceptedRequests: map[string]diagnostic.AcceptedRequest{}, StartRequestDigest: "start-digest-a",
	}
	stored, reused, err := store.Create(ctx, want.OwnerID, "start-key", want.StartRequestDigest, want)
	if err != nil || reused {
		t.Fatalf("create diagnostic: reused=%v err=%v", reused, err)
	}
	if !reflect.DeepEqual(stored, want) {
		t.Fatalf("created diagnostic differs: got=%#v want=%#v", stored, want)
	}
	var subjectID, subjectName string
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT subject_id,subject_name_snapshot,map_revision FROM diagnostic_sessions WHERE id=$1`, want.ID).Scan(&subjectID, &subjectName, &revision); err != nil {
		t.Fatal(err)
	}
	if subjectID != "subject:intro-to-ml" || subjectName != "Введение в ML" || revision != want.Variant.MapRevision {
		t.Fatalf("session subject snapshot=%q/%q revision=%d", subjectID, subjectName, revision)
	}

	restarted := New(pool)
	loaded, err := restarted.Get(ctx, want.OwnerID, want.ID)
	if err != nil || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("read after repository restart: loaded=%#v err=%v", loaded, err)
	}
	byKey, found, err := restarted.FindStart(ctx, want.OwnerID, "start-key", want.StartRequestDigest)
	if err != nil || !found || !reflect.DeepEqual(byKey, want) {
		t.Fatalf("find idempotent start: found=%v loaded=%#v err=%v", found, byKey, err)
	}
	duplicate, reused, err := restarted.Create(ctx, want.OwnerID, "start-key", want.StartRequestDigest, want)
	if err != nil || !reused || duplicate.ID != want.ID {
		t.Fatalf("duplicate start: id=%s reused=%v err=%v", duplicate.ID, reused, err)
	}

	if _, found, err := restarted.FindStart(ctx, want.OwnerID, "start-key", "different-digest"); !isKind(err, fault.Conflict) || found {
		t.Fatalf("changed start digest: found=%v err=%v", found, err)
	}
	if _, _, err := restarted.Create(ctx, want.OwnerID, "start-key", "different-digest", want); !isKind(err, fault.Conflict) {
		t.Fatalf("create with reused key and changed digest: %v", err)
	}
	if _, err := restarted.Get(ctx, "diag-owner-b", want.ID); !isKind(err, fault.NotFound) {
		t.Fatalf("other owner read diagnostic: %v", err)
	}
	otherOwner := diagnostic.Session{ID: "diagnostic-b", OwnerID: "diag-owner-b", VariantID: "variant-b", Status: diagnostic.StatusActive,
		Variant: diagnostic.VariantSnapshot{ID: "variant-b", AlgorithmVersion: "test", IncludedCompetencyCount: 1},
		AcceptedRequests: map[string]diagnostic.AcceptedRequest{}, StartRequestDigest: "owner-b-digest"}
	ownerBSession, reused, err := restarted.Create(ctx, otherOwner.OwnerID, "start-key", otherOwner.StartRequestDigest, otherOwner)
	if err != nil || reused || ownerBSession.ID != otherOwner.ID {
		t.Fatalf("same start key for second owner: session=%#v reused=%v err=%v", ownerBSession, reused, err)
	}
}

func isKind(err error, kind fault.Kind) bool {
	var failure *fault.Error
	return errors.As(err, &failure) && failure.Kind == kind
}
