package postgres

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audiopreparation"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LearningAudioProgress struct{ pool *pgxpool.Pool }

func NewLearningAudioProgress(pool *pgxpool.Pool) *LearningAudioProgress {
	return &LearningAudioProgress{pool: pool}
}

func (r *LearningAudioProgress) Snapshot(ctx context.Context, subjectID string) (int64, []audiopreparation.LearningAsset, error) {
	rows, err := db.New(r.pool).LearningAudioPreparationForSubject(ctx, subjectID)
	if err != nil {
		return 0, nil, err
	}
	assets := make([]audiopreparation.LearningAsset, 0, len(rows))
	var revision int64
	for _, row := range rows {
		revision = row.Revision
		if row.TaskID == nil {
			continue
		}
		asset := audiopreparation.LearningAsset{TaskID: *row.TaskID, Attempts: int(valueInt32(row.Attempts))}
		if row.VoiceInstruction != nil {
			asset.Instruction = *row.VoiceInstruction
		}
		asset.AssetID = row.AudioAssetID
		asset.Status = row.Status
		if row.NextAttemptAt.Valid {
			value := row.NextAttemptAt.Time
			asset.NextAttemptAt = &value
		}
		if row.LeaseUntil.Valid {
			value := row.LeaseUntil.Time
			asset.LeaseUntil = &value
		}
		assets = append(assets, asset)
	}
	return revision, assets, nil
}

func valueInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

var _ interface {
	Snapshot(context.Context, string) (int64, []audiopreparation.LearningAsset, error)
} = (*LearningAudioProgress)(nil)
