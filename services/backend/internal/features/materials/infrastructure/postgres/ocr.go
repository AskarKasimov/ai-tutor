package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimOCR(ctx context.Context, token string, now time.Time) (application.OCRJob, error) {
	return r.ClaimOCRReady(ctx, token, now, true)
}

func (r *Repository) ClaimOCRReady(ctx context.Context, token string, now time.Time, allowPDF bool) (application.OCRJob, error) {
	job, err := db.New(r.pool).ClaimMaterialOCR(ctx, db.ClaimMaterialOCRParams{
		LeaseToken: token,
		Now:        now.Unix(),
		AllowPdf:   allowPDF,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.OCRJob{}, nil
	}
	return application.OCRJob{MaterialID: job.MaterialID, Token: job.LeaseToken, Attempts: int(job.Attempts)}, err
}

func (r *Repository) RenewOCR(ctx context.Context, job application.OCRJob, now time.Time) (bool, error) {
	n, err := db.New(r.pool).RenewMaterialOCR(ctx, db.RenewMaterialOCRParams{MaterialID: job.MaterialID, LeaseToken: job.Token, Now: now.Unix()})
	return n == 1, err
}

func (r *Repository) ReleaseOCR(ctx context.Context, job application.OCRJob, now time.Time) error {
	_, err := db.New(r.pool).ReleaseMaterialOCR(ctx, db.ReleaseMaterialOCRParams{MaterialID: job.MaterialID, LeaseToken: job.Token, Now: now.Unix()})
	return err
}

func (r *Repository) SetOCRStage(ctx context.Context, job application.OCRJob, stage string, now time.Time) (bool, error) {
	n, err := db.New(r.pool).SetMaterialOCRStage(ctx, db.SetMaterialOCRStageParams{MaterialID: job.MaterialID, LeaseToken: job.Token, Stage: stage, Now: now.Unix()})
	return n == 1, err
}

func (r *Repository) FinishOCR(ctx context.Context, job application.OCRJob, ocrKey, parsedKey string, report json.RawMessage, text string, warning *string, now time.Time) (bool, error) {
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
	updated, err := q.UpdateSubjectMaterialOCR(ctx, db.UpdateSubjectMaterialOCRParams{
		MaterialID:   job.MaterialID,
		S3ParsedKey:  parsedKey,
		S3OcrKey:     ocrKey,
		OcrReport:    report,
		ParsedText:   text,
		CharCount:    int32(utf8.RuneCountInString(text)),
		ErrorMessage: warning,
		Now:          now.Unix(),
	})
	if err != nil {
		return false, err
	}
	if updated != 1 {
		return false, nil
	}
	finished, err := q.FinishMaterialOCR(ctx, db.FinishMaterialOCRParams{MaterialID: job.MaterialID, LeaseToken: job.Token, UpdatedAt: now.Unix()})
	if err != nil {
		return false, err
	}
	if finished != 1 {
		return false, nil
	}
	if err := q.EnqueueMaterialCleaning(ctx, db.EnqueueMaterialCleaningParams{MaterialID: job.MaterialID, UpdatedAt: now.Unix()}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) FailOCR(ctx context.Context, job application.OCRJob, message string, now time.Time) error {
	return db.New(r.pool).FailMaterialOCR(ctx, db.FailMaterialOCRParams{
		MaterialID:   job.MaterialID,
		LeaseToken:   job.Token,
		ErrorMessage: message,
		AvailableAt:  now.Add(time.Duration(job.Attempts) * 30 * time.Second).Unix(),
		Now:          now.Unix(),
	})
}

var _ application.OCRQueue = (*Repository)(nil)
