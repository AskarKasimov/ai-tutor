package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

func toCleaning(j db.MaterialCleaningJob) material.Cleaning {
	return material.Cleaning{Status: j.Status, Attempts: int(j.Attempts), CleanedText: j.CleanedText,
		S3CleanedKey: j.S3CleanedKey, S3ReportKey: j.S3ReportKey, ErrorMessage: j.ErrorMessage, UpdatedAt: j.UpdatedAt}
}
func (r *Repository) ClaimCleaning(ctx context.Context, token string, now time.Time) (material.CleaningJob, error) {
	j, err := db.New(r.pool).ClaimMaterialCleaning(ctx, db.ClaimMaterialCleaningParams{LeaseToken: token, Now: now.Unix()})
	if errors.Is(err, pgx.ErrNoRows) {
		return material.CleaningJob{}, nil
	}
	return material.CleaningJob{MaterialID: j.MaterialID, Token: j.LeaseToken, Attempts: int(j.Attempts)}, err
}

func (r *Repository) RenewCleaning(ctx context.Context, job material.CleaningJob, now time.Time) (bool, error) {
	n, err := db.New(r.pool).RenewMaterialCleaning(ctx, db.RenewMaterialCleaningParams{MaterialID: job.MaterialID, LeaseToken: job.Token, Now: now.Unix()})
	return n == 1, err
}

func (r *Repository) ReleaseCleaning(ctx context.Context, job material.CleaningJob, now time.Time) error {
	_, err := db.New(r.pool).ReleaseMaterialCleaning(ctx, db.ReleaseMaterialCleaningParams{MaterialID: job.MaterialID, LeaseToken: job.Token, Now: now.Unix()})
	return err
}
func (r *Repository) FinishCleaning(ctx context.Context, job material.CleaningJob, result application.CleaningResult, textKey, reportKey string, now time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if _, err = q.LockSubjectMaterial(ctx, job.MaterialID); errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	n, err := q.FinishMaterialCleaning(ctx, db.FinishMaterialCleaningParams{MaterialID: job.MaterialID, LeaseToken: job.Token, CleanedText: result.Markdown, S3CleanedKey: textKey, S3ReportKey: reportKey, UpdatedAt: now.Unix()})
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return n == 1, nil
}
func (r *Repository) FailCleaning(ctx context.Context, job material.CleaningJob, message string, now time.Time) error {
	_, err := db.New(r.pool).FailMaterialCleaning(ctx, db.FailMaterialCleaningParams{MaterialID: job.MaterialID, LeaseToken: job.Token, ErrorMessage: message, AvailableAt: now.Add(time.Duration(job.Attempts) * 30 * time.Second).Unix(), UpdatedAt: now.Unix()})
	return err
}
