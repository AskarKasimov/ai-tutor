package postgres

import (
	"context"
	"fmt"

	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

// EnqueueTaskAudio links a task to its immutable instruction asset inside the
// caller's transaction. It does not create or commit a transaction itself.
func EnqueueTaskAudio(ctx context.Context, q *db.Queries, taskID, assetID, objectKey, instruction string) (*string, error) {
	storedInstruction, err := q.InsertTaskAudioAsset(ctx, db.InsertTaskAudioAssetParams{
		ID: assetID, Instruction: instruction, ObjectKey: objectKey,
	})
	if err != nil {
		return nil, fmt.Errorf("insert task audio asset: %w", err)
	}
	if storedInstruction != instruction {
		return nil, fmt.Errorf("task audio asset %q has a different immutable instruction", assetID)
	}
	rows, err := q.LinkTaskAudioAsset(ctx, db.LinkTaskAudioAssetParams{ID: taskID, AudioAssetID: &assetID})
	if err != nil {
		return nil, fmt.Errorf("link task audio asset: %w", err)
	}
	if rows != 1 {
		return nil, fmt.Errorf("task %q not found while linking task audio asset", taskID)
	}
	return &assetID, nil
}
