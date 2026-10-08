package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: db.New(pool)}
}

func (r *Repository) Enqueue(ctx context.Context, id, instruction, objectKey string) error {
	return r.q.EnqueueTaskAudio(ctx, db.EnqueueTaskAudioParams{ID: id, Instruction: instruction, ObjectKey: objectKey})
}

func (r *Repository) Claim(ctx context.Context, token string, lease time.Duration) (application.Claim, bool, error) {
	if token == "" || lease <= 0 {
		return application.Claim{}, false, errors.New("claim token and positive lease are required")
	}
	if err := r.q.RecoverExpiredTaskAudio(ctx); err != nil {
		return application.Claim{}, false, fmt.Errorf("recover expired task audio leases: %w", err)
	}
	row, err := r.q.ClaimTaskAudio(ctx, db.ClaimTaskAudioParams{
		ClaimToken: &token, LeaseSeconds: lease.Seconds(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.Claim{}, false, nil
	}
	if err != nil {
		return application.Claim{}, false, fmt.Errorf("claim task audio: %w", err)
	}
	return application.Claim{Asset: assetFromRow(row.ID, row.Instruction, row.ObjectKey, row.Bucket, row.StorageUri, row.AudioUrl, row.Status, row.Attempts), Token: token}, true, nil
}

func (r *Repository) IsCurrent(ctx context.Context, claim application.Claim) (bool, error) {
	return r.q.IsCurrentTaskAudio(ctx, db.IsCurrentTaskAudioParams{ID: claim.Asset.ID, ClaimToken: &claim.Token})
}

func (r *Repository) Cancel(ctx context.Context, claim application.Claim) (bool, error) {
	rows, err := r.q.CancelClaimedTaskAudio(ctx, db.CancelClaimedTaskAudioParams{ID: claim.Asset.ID, ClaimToken: &claim.Token})
	if err != nil {
		return false, fmt.Errorf("cancel stale task audio: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) Complete(ctx context.Context, claim application.Claim, bucket, storageURI, audioURL string) (bool, error) {
	rows, err := r.q.CompleteTaskAudio(ctx, db.CompleteTaskAudioParams{
		ID: claim.Asset.ID, ClaimToken: &claim.Token, Bucket: &bucket, StorageUri: &storageURI, AudioUrl: &audioURL,
	})
	if err != nil {
		return false, fmt.Errorf("complete task audio: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) Fail(ctx context.Context, claim application.Claim, code string, delay time.Duration) (bool, error) {
	if code == "" || delay < 0 {
		return false, errors.New("error code and non-negative delay are required")
	}
	rows, err := r.q.FailTaskAudio(ctx, db.FailTaskAudioParams{
		ID: claim.Asset.ID, ClaimToken: &claim.Token, ErrorCode: &code, DelaySeconds: delay.Seconds(),
	})
	if err != nil {
		return false, fmt.Errorf("fail task audio: %w", err)
	}
	return rows == 1, nil
}

func (r *Repository) Get(ctx context.Context, assetID string) (audioasset.Asset, error) {
	row, err := r.q.GetTaskAudio(ctx, assetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return audioasset.Asset{}, audioasset.ErrNotFound
	}
	if err != nil {
		return audioasset.Asset{}, err
	}
	return assetFromRow(row.ID, row.Instruction, row.ObjectKey, row.Bucket, row.StorageUri, row.AudioUrl, row.Status, row.Attempts), nil
}

func (r *Repository) GetAccessible(ctx context.Context, ownerID, assetID string) (audioasset.Asset, error) {
	row, err := r.q.GetAccessibleTaskAudio(ctx, db.GetAccessibleTaskAudioParams{UserID: ownerID, ID: assetID})
	if errors.Is(err, pgx.ErrNoRows) {
		return audioasset.Asset{}, audioasset.ErrNotFound
	}
	if err != nil {
		return audioasset.Asset{}, err
	}
	return assetFromRow(row.ID, row.Instruction, row.ObjectKey, row.Bucket, row.StorageUri, row.AudioUrl, row.Status, row.Attempts), nil
}

func assetFromRow(id, instruction, objectKey string, bucket, storageURI, audioURL *string, status string, attempts int32) audioasset.Asset {
	return audioasset.Asset{
		ID: id, Instruction: instruction, ObjectKey: objectKey, Bucket: bucket, StorageURI: storageURI,
		AudioURL: audioURL, Status: audioasset.Status(status), Attempts: int(attempts),
	}
}

var _ application.Queue = (*Repository)(nil)
var _ application.AssetReader = (*Repository)(nil)
