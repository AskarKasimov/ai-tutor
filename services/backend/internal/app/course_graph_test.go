package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	graphpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/infrastructure/postgres"
	materialsapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	materialspg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/postgres"
)

func TestCourseGraphQueueSnapshotsAndStaleWorkers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()
	materials := materialspg.New(f.pool)
	repo := graphpg.New(f.pool)
	m := material.Material{ID: "graph_material", SubjectID: "subject:test", Filename: "lecture.md", S3RawKey: "raw", S3ParsedKey: "parsed", ParsedText: "Теория", Status: material.StatusParsed, CreatedAt: 1, UpdatedAt: 1}
	if _, err := materials.Insert(ctx, m); err != nil {
		t.Fatal(err)
	}
	if job, err := repo.Claim(ctx, "early", now); err != nil || job.SubjectID != "" {
		t.Fatalf("claimed before cleaning: %+v %v", job, err)
	}
	clean, err := materials.ClaimCleaning(ctx, "clean", now)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := materials.FinishCleaning(ctx, clean, materialsapp.CleaningResult{Markdown: "Теория"}, "text", "report", now); err != nil || !ok {
		t.Fatalf("clean: %v %v", ok, err)
	}
	first, err := repo.Claim(ctx, "first", now)
	if err != nil || first.SubjectID != m.SubjectID {
		t.Fatalf("claim: %+v %v", first, err)
	}
	if job, err := repo.Claim(ctx, "double", now); err != nil || job.SubjectID != "" {
		t.Fatalf("double claim: %+v %v", job, err)
	}
	if owned, err := repo.Renew(ctx, first, now.Add(30*time.Second)); err != nil || !owned {
		t.Fatalf("heartbeat renew: owned=%v err=%v", owned, err)
	}
	if job, err := repo.Claim(ctx, "before-renewed-expiry", now.Add(61*time.Second)); err != nil || job.SubjectID != "" {
		t.Fatalf("heartbeat did not extend graph lease: %+v %v", job, err)
	}
	next, err := repo.Claim(ctx, "next", now.Add(91*time.Second))
	if err != nil || next.Token != "next" {
		t.Fatalf("lease: %+v %v", next, err)
	}
	graph := json.RawMessage(`{"version":"course-graph-v1","topics":[{"id":"t1"}],"prerequisites":[],"quality":{"acyclic":true}}`)
	if ok, err := repo.Finish(ctx, first, graph, now); err != nil || ok {
		t.Fatalf("stale accepted: %v %v", ok, err)
	}
	if ok, err := repo.Finish(ctx, next, graph, now); err != nil || !ok {
		t.Fatalf("finish: %v %v", ok, err)
	}
	snapshot, err := repo.Read(ctx, m.SubjectID)
	if err != nil || snapshot.Stale || snapshot.Status != "ready" {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	// A new upload invalidates the old graph immediately, but preserves it.
	m.ID = "new_material"
	if _, err := materials.Insert(ctx, m); err != nil {
		t.Fatal(err)
	}
	snapshot, err = repo.Read(ctx, m.SubjectID)
	if err != nil || !snapshot.Stale || len(snapshot.Graph) == 0 {
		t.Fatalf("previous graph lost: %+v %v", snapshot, err)
	}
	clean, err = materials.ClaimCleaning(ctx, "clean2", now)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := materials.FinishCleaning(ctx, clean, materialsapp.CleaningResult{Markdown: "Теория"}, "text2", "report2", now); err != nil || !ok {
		t.Fatal(err)
	}
	changed, err := repo.Claim(ctx, "changed", now)
	if err != nil || changed.Fingerprint == next.Fingerprint {
		t.Fatalf("new input not queued: %+v %v", changed, err)
	}
	if ok, err := repo.Finish(ctx, next, graph, now); err != nil || ok {
		t.Fatalf("old fingerprint accepted: %v %v", ok, err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if changed.Attempts != attempt {
			t.Fatalf("attempt=%d, want %d", changed.Attempts, attempt)
		}
		if err := repo.Fail(ctx, changed, "failure", now); err != nil {
			t.Fatal(err)
		}
		if attempt < 3 {
			changed, err = repo.Claim(ctx, "retry", now.Add(time.Duration(attempt)*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	snapshot, err = repo.Read(ctx, m.SubjectID)
	if err != nil || snapshot.Status != "failed" || len(snapshot.Graph) == 0 {
		t.Fatalf("failed build erased graph: %+v %v", snapshot, err)
	}
}
