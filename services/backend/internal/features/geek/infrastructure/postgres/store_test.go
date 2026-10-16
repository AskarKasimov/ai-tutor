package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	shared "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

func storePool(t *testing.T) *pgxpool.Pool {
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
	schema := "geek_test_" + hex.EncodeToString(suffix)
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
		`INSERT INTO users(id, email, password_hash, created_at) VALUES ('u', 'u@example.edu', 'x', 1)`,
		`INSERT INTO subjects(id, name, created_at) VALUES ('s', 'Курс', 1)`,
		`INSERT INTO transcriptions(id, user_id, text, created_at) VALUES ('tr', 'u', 'Ответ голосом', 1)`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func session(id string) exam.Session {
	g := exam.Graph{Version: "v1", Nodes: []exam.Node{{ID: "n", Title: "Тема"}}}
	e, _ := exam.NewExam(g, 0)
	return exam.Session{ID: id, OwnerID: "u", SubjectID: "s", Status: exam.SessionActive, StartedAt: 1, Version: 1, Exam: e, Turns: []exam.Turn{}}
}

func TestStoreKeepsOneActiveSessionAndVersionsUpdates(t *testing.T) {
	store := New(storePool(t))
	ctx := context.Background()
	if ok, err := store.SubjectExists(ctx, "s"); err != nil || !ok {
		t.Fatalf("subject: %v %v", ok, err)
	}
	first := session("d1")
	if err := store.Create(ctx, first, application.Request{OwnerID: "u", Key: "k1", Fingerprint: "f", SessionID: "d1", Created: true, Response: first}); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, session("d2"), application.Request{OwnerID: "u", Key: "k2", SessionID: "d2", Response: first}); !errors.Is(err, application.ErrActiveExists) {
		t.Fatalf("second active: %v", err)
	}
	active, ok, err := store.ActiveSession(ctx, "u", "s")
	if err != nil || !ok || active.ID != "d1" || active.Exam.Graph.Version != "v1" {
		t.Fatalf("active: %+v %v %v", active.ID, ok, err)
	}
	if r, ok, err := store.Request(ctx, "u", "k1"); err != nil || !ok || !r.Created || r.Response.ID != "d1" {
		t.Fatalf("request: %+v %v %v", r, ok, err)
	}
	done := first
	done.Status, done.Version = exam.SessionCompleted, 2
	if err := store.Update(ctx, done, 1, &application.Request{OwnerID: "u", Key: "k3", Fingerprint: "a", SessionID: "d1", Response: done}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, done, 1, nil); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if got, err := store.Session(ctx, "u", "d1"); err != nil || got.Status != exam.SessionCompleted || got.Version != 2 {
		t.Fatalf("session: %+v %v", got.Status, err)
	}
	if _, err := store.Session(ctx, "other", "d1"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("foreign session: %v", err)
	}
	// The completed session no longer blocks a new active one.
	if err := store.Create(ctx, session("d2"), application.Request{OwnerID: "u", Key: "k2", SessionID: "d2", Response: first}); err != nil {
		t.Fatal(err)
	}
	// SaveRequest keeps the first request stored under a key.
	resumed := application.Request{OwnerID: "u", Key: "k4", Fingerprint: "start:s", SessionID: "d2", Response: first}
	if err := store.SaveRequest(ctx, resumed); err != nil {
		t.Fatal(err)
	}
	resumed.Fingerprint = "other"
	if err := store.SaveRequest(ctx, resumed); err != nil {
		t.Fatal(err)
	}
	if r, ok, err := store.Request(ctx, "u", "k4"); err != nil || !ok || r.Fingerprint != "start:s" || r.Created {
		t.Fatalf("saved request: %+v %v %v", r, ok, err)
	}
	if list, err := store.Sessions(ctx, "u", "s"); err != nil || len(list) != 2 {
		t.Fatalf("sessions: %d %v", len(list), err)
	}
	texts := NewTranscriptions(store.pool)
	if text, ok, err := texts.Text(ctx, "u", "tr"); err != nil || !ok || text != "Ответ голосом" {
		t.Fatalf("transcription: %q %v %v", text, ok, err)
	}
	if _, ok, _ := texts.Text(ctx, "other", "tr"); ok {
		t.Fatal("foreign transcription read")
	}
}
