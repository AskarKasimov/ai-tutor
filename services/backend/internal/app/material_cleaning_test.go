package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	materialsapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	materialspg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/postgres"
)

type fixtureCleaner struct{ fail bool }

func (c fixtureCleaner) Clean(_ context.Context, source string) (materialsapp.CleaningResult, error) {
	if c.fail {
		return materialsapp.CleaningResult{}, errors.New("unavailable")
	}
	hash := sha256.Sum256([]byte(source))
	report, _ := json.Marshal(map[string]any{"version": "material-cleaning-v1", "source_sha256": hex.EncodeToString(hash[:]), "removed": []any{}})
	return materialsapp.CleaningResult{Markdown: source, Report: report}, nil
}

func TestMaterialCleaningDurableQueue(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repo := materialspg.New(f.pool)
	source := "# Лекция\n\nТеория $E=mc^2$"
	m := material.Material{ID: "mat_clean", SubjectID: "subject:test", Filename: "lecture.md", S3RawKey: "raw", S3ParsedKey: "parsed", ParsedText: source, Status: material.StatusParsed, CreatedAt: 1, UpdatedAt: 1}
	if _, err := repo.Insert(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := f.app.materialsStorage.Put(ctx, "parsed", "text/markdown", []byte(source)); err != nil {
		t.Fatal(err)
	}
	// A fresh repository/worker consumes the persisted task without in-memory enqueue.
	worker := materialsapp.NewCleaningWorker(materialspg.New(f.pool), f.app.materialsStorage, fixtureCleaner{})
	if worked, err := worker.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("process: %v %v", worked, err)
	}
	got, err := repo.Get(ctx, m.ID)
	if err != nil || got.Cleaning == nil || got.Cleaning.Status != "ready" || got.Cleaning.CleanedText != source || got.ParsedText != source {
		t.Fatalf("result: %+v %v", got, err)
	}
	if _, err = f.app.materialsStorage.Get(ctx, got.Cleaning.S3CleanedKey); err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.Delete(ctx, m.SubjectID, m.ID)
	if err != nil || deleted.Cleaning == nil || deleted.Cleaning.S3CleanedKey != got.Cleaning.S3CleanedKey {
		t.Fatalf("delete lost output keys: %+v %v", deleted, err)
	}
}

func TestMaterialCleaningLeaseAndRetryLimit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repo := materialspg.New(f.pool)
	m := material.Material{ID: "mat_retry", SubjectID: "subject:test", Filename: "lecture.md", S3RawKey: "raw", S3ParsedKey: "parsed", ParsedText: "Теория", Status: material.StatusParsed, CreatedAt: 1, UpdatedAt: 1}
	if _, err := repo.Insert(ctx, m); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	first, err := repo.ClaimCleaning(ctx, "first", now)
	if err != nil || first.MaterialID == "" {
		t.Fatal(err)
	}
	if second, err := repo.ClaimCleaning(ctx, "second", now); err != nil || second.MaterialID != "" {
		t.Fatalf("double claim: %+v %v", second, err)
	}
	next, err := repo.ClaimCleaning(ctx, "next", now.Add(8*time.Minute))
	if err != nil || next.Token != "next" {
		t.Fatalf("expired lease: %+v %v", next, err)
	}
	if ok, err := repo.FinishCleaning(ctx, first, materialsapp.CleaningResult{}, "old", "old", now); err != nil || ok {
		t.Fatalf("stale completion accepted: %v %v", ok, err)
	}
	if err = repo.FailCleaning(ctx, next, "retry", now); err != nil {
		t.Fatal(err)
	}
	final, err := repo.ClaimCleaning(ctx, "final", now.Add(10*time.Minute))
	if err != nil || final.Attempts != 3 {
		t.Fatalf("third attempt: %+v %v", final, err)
	}
	if err = repo.FailCleaning(ctx, final, "failed", now); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, m.ID)
	if err != nil || got.Cleaning.Status != "failed" || got.ParsedText != "Теория" {
		t.Fatalf("failure lost source: %+v %v", got, err)
	}
}
