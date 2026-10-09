package postgres

import (
	"context"
	"testing"

	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

func TestEnqueueTaskAudioRejectsInstructionCollisionWithoutChangingAsset(t *testing.T) {
	pool := migrationPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subjects(id,name,created_at) VALUES ('subject:test','Тестовый предмет',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO audio_assets(id,instruction,object_key) VALUES ('collision','Старая инструкция','task-audio/v1/collision.wav')`); err != nil {
		t.Fatal(err)
	}
	if _, err := EnqueueTaskAudio(ctx, db.New(pool), "missing-task", "collision", "task-audio/v1/collision.wav", "Другая инструкция"); err == nil {
		t.Fatal("enqueue accepted a different instruction for an existing asset")
	}
	var instruction string
	if err := pool.QueryRow(ctx, `SELECT instruction FROM audio_assets WHERE id='collision'`).Scan(&instruction); err != nil || instruction != "Старая инструкция" {
		t.Fatalf("immutable instruction changed to %q: %v", instruction, err)
	}
}
