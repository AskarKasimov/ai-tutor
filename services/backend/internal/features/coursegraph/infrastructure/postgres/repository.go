package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool} }
func (r *Repository) Claim(ctx context.Context, token string, now time.Time) (coursegraph.Job, error) {
	q := db.New(r.pool)
	if err := q.SyncCourseGraphJobs(ctx, now.Unix()); err != nil {
		return coursegraph.Job{}, err
	}
	job, err := q.ClaimCourseGraph(ctx, db.ClaimCourseGraphParams{Token: token, Now: now.Unix()})
	if errors.Is(err, pgx.ErrNoRows) {
		return coursegraph.Job{}, nil
	}
	return coursegraph.Job{SubjectID: job.SubjectID, Fingerprint: job.Fingerprint, Token: job.LeaseToken, Attempts: int(job.Attempts)}, err
}

func (r *Repository) Renew(ctx context.Context, job coursegraph.Job, now time.Time) (bool, error) {
	n, err := db.New(r.pool).RenewCourseGraph(ctx, db.RenewCourseGraphParams{SubjectID: job.SubjectID, Token: job.Token, Now: now.Unix()})
	return n == 1, err
}

func (r *Repository) Release(ctx context.Context, job coursegraph.Job, now time.Time) error {
	_, err := db.New(r.pool).ReleaseCourseGraph(ctx, db.ReleaseCourseGraphParams{SubjectID: job.SubjectID, Token: job.Token, Now: now.Unix()})
	return err
}
func (r *Repository) Finish(ctx context.Context, job coursegraph.Job, graph json.RawMessage, now time.Time) (bool, error) {
	_, err := db.New(r.pool).FinishCourseGraph(ctx, db.FinishCourseGraphParams{SubjectID: job.SubjectID, Fingerprint: job.Fingerprint, Token: job.Token, Graph: graph, Now: now.Unix()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
func (r *Repository) Fail(ctx context.Context, job coursegraph.Job, message string, now time.Time) error {
	return db.New(r.pool).FailCourseGraph(ctx, db.FailCourseGraphParams{SubjectID: job.SubjectID, Token: job.Token, ErrorMessage: message, Now: now.Unix()})
}
func (r *Repository) Read(ctx context.Context, subjectID string) (coursegraph.Snapshot, error) {
	row, err := db.New(r.pool).ReadCourseGraph(ctx, subjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return coursegraph.Snapshot{}, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Предмет не найден.")
	}
	if err != nil {
		return coursegraph.Snapshot{}, err
	}
	return coursegraph.Snapshot{Status: row.Status, Attempts: row.Attempts, ErrorMessage: row.ErrorMessage, Graph: row.Graph, UpdatedAt: row.UpdatedAt, Stale: row.Stale}, nil
}

func (r *Repository) RetryFailedBuild(ctx context.Context, subjectID string, now time.Time) (bool, error) {
	n, err := db.New(r.pool).RetryFailedCourseGraphBuild(ctx, db.RetryFailedCourseGraphBuildParams{
		SubjectID:   subjectID,
		AvailableAt: now.Unix(),
	})
	return n > 0, err
}
