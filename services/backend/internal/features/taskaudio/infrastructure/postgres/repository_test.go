package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	sharedpostgres "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupAudioRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
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
	schema := "task_audio_test_" + hex.EncodeToString(suffix)
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
	if err := sharedpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return New(pool), pool, ctx
}

func seedCurrentTaskAudio(t *testing.T, pool *pgxpool.Pool, ctx context.Context, assetID, taskID string) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO users(id,email,password_hash,created_at) VALUES ('audio-owner','audio-owner@example.test','test',1)", nil},
		{"INSERT INTO competency_map_imports(revision,imported_at,imported_by,competency_count,constituent_count,outcome_count,task_count,source_format,source_headers) VALUES (1,1,'audio-owner',1,1,1,1,'paired','[]')", nil},
		{"INSERT INTO competencies(id,name,revision) VALUES ('audio-competency','Audio',1)", nil},
		{"INSERT INTO constituents(id,competency_id,name) VALUES ('audio-constituent','audio-competency','Audio')", nil},
		{"INSERT INTO outcomes(id,constituent_id,name) VALUES ('audio-outcome','audio-constituent','Audio')", nil},
		{"INSERT INTO audio_assets(id,instruction,object_key) VALUES ($1,'Speak answer',$2)", []any{assetID, "task-audio/v1/" + assetID + ".wav"}},
		{"INSERT INTO tasks(id,outcome_id,question,voice_instruction,created_at,audio_asset_id) VALUES ($1,'audio-outcome','Question','Speak answer',1,$2)", []any{taskID, assetID}},
		{"UPDATE competency_map_state SET revision=1 WHERE singleton=true", nil},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed current audio task: %v", err)
		}
	}
}

func TestClaimIsExclusiveAndStaleClaimCannotComplete(t *testing.T) {
	repository, pool, ctx := setupAudioRepository(t)
	seedCurrentTaskAudio(t, pool, ctx, "asset-1", "task-1")
	first, ok, err := repository.Claim(ctx, "token-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	_, ok, err = repository.Claim(ctx, "token-2", time.Minute)
	if err != nil || ok {
		t.Fatalf("second claim: ok=%v err=%v, want no available claim", ok, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audio_assets SET lease_until=now()-interval '1 second' WHERE id='asset-1'`); err != nil {
		t.Fatal(err)
	}
	second, ok, err := repository.Claim(ctx, "token-2", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaimed expired lease: ok=%v err=%v", ok, err)
	}
	completed, err := repository.Complete(ctx, first, "bucket", "s3://bucket/key", "/task-audio/asset-1/file")
	if err != nil || completed {
		t.Fatalf("stale claim completed: completed=%v err=%v", completed, err)
	}
	completed, err = repository.Complete(ctx, second, "bucket", "s3://bucket/key", "/task-audio/asset-1/file")
	if err != nil || !completed {
		t.Fatalf("current claim completion: completed=%v err=%v", completed, err)
	}
	asset, err := repository.Get(ctx, "asset-1")
	if err != nil || asset.Status != "ready" || asset.AudioURL == nil {
		t.Fatalf("ready asset: %#v err=%v", asset, err)
	}
}

func TestClaimRejectsStaleTaskAndExpiryCancelsIt(t *testing.T) {
	repository, pool, ctx := setupAudioRepository(t)
	seedCurrentTaskAudio(t, pool, ctx, "asset-stale", "task-stale")
	claim, ok, err := repository.Claim(ctx, "token-stale", time.Minute)
	if err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE competency_map_state SET revision=2 WHERE singleton=true`); err != nil {
		t.Fatal(err)
	}
	if current, err := repository.IsCurrent(ctx, claim); err != nil || current {
		t.Fatalf("stale claim current=%v err=%v", current, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audio_assets SET lease_until=now()-interval '1 second' WHERE id='asset-stale'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repository.Claim(ctx, "token-next", time.Minute); err != nil || ok {
		t.Fatalf("stale asset claimed after recovery: ok=%v err=%v", ok, err)
	}
	asset, err := repository.Get(ctx, "asset-stale")
	if err != nil || asset.Status != "cancelled" {
		t.Fatalf("expired stale asset status=%q err=%v", asset.Status, err)
	}
	if cancelled, err := repository.Cancel(ctx, claim); err != nil || cancelled {
		t.Fatalf("expired worker cancelled after lease recovery: cancelled=%v err=%v", cancelled, err)
	}
}

func TestFailureAfterReplacementCancelsInsteadOfRequeueing(t *testing.T) {
	repository, pool, ctx := setupAudioRepository(t)
	seedCurrentTaskAudio(t, pool, ctx, "asset-replaced", "task-replaced")
	claim, ok, err := repository.Claim(ctx, "token-replaced", time.Minute)
	if err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE competency_map_state SET revision=2 WHERE singleton=true`); err != nil {
		t.Fatal(err)
	}
	if failed, err := repository.Fail(ctx, claim, "synthesis_failed", time.Second); err != nil || !failed {
		t.Fatalf("record failed stale attempt: failed=%v err=%v", failed, err)
	}
	asset, err := repository.Get(ctx, "asset-replaced")
	if err != nil || asset.Status != "cancelled" {
		t.Fatalf("stale failure requeued asset: status=%q err=%v", asset.Status, err)
	}
	if _, ok, err := repository.Claim(ctx, "token-retry", time.Minute); err != nil || ok {
		t.Fatalf("cancelled stale asset was claimed: ok=%v err=%v", ok, err)
	}
}

func TestExpiredLeaseOnCurrentTaskIsRecoverableNotCancelled(t *testing.T) {
	repository, pool, ctx := setupAudioRepository(t)
	seedCurrentTaskAudio(t, pool, ctx, "asset-current-expired", "task-current-expired")
	claim, ok, err := repository.Claim(ctx, "token-expired", time.Minute)
	if err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audio_assets SET lease_until=now()-interval '1 second' WHERE id='asset-current-expired'`); err != nil {
		t.Fatal(err)
	}
	if current, err := repository.IsCurrent(ctx, claim); err != nil || current {
		t.Fatalf("expired lease check: current=%v err=%v", current, err)
	}
	if cancelled, err := repository.Cancel(ctx, claim); err != nil || cancelled {
		t.Fatalf("active map task was cancelled for expired lease: cancelled=%v err=%v", cancelled, err)
	}
	reclaimed, ok, err := repository.Claim(ctx, "token-reclaimed", time.Minute)
	if err != nil || !ok || reclaimed.Token != "token-reclaimed" {
		t.Fatalf("current task was not recoverable: claim=%+v ok=%v err=%v", reclaimed, ok, err)
	}
}
