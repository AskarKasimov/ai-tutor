package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct{ queries *db.Queries }

func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool)} }

func (r *Repository) Create(ctx context.Context, id, name string, createdAt int64) (subject.Subject, error) {
	created, err := r.queries.CreateSubject(ctx, db.CreateSubjectParams{ID: id, Name: name, CreatedAt: createdAt})
	if err != nil {
		return subject.Subject{}, err
	}
	return subject.Subject{ID: created.ID, Name: created.Name}, nil
}

func (r *Repository) AudioPreparationState(ctx context.Context, subjectID string) (bool, bool, error) {
	state, err := r.queries.SubjectAudioPreparationState(ctx, subjectID)
	if err != nil {
		return false, false, err
	}
	return state.SubjectExists, state.GraphPublished, nil
}

func (r *Repository) List(ctx context.Context) ([]subject.Subject, error) {
	rows, err := r.queries.ListSubjects(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]subject.Subject, 0, len(rows))
	for _, row := range rows {
		items = append(items, subject.Subject{
			ID: row.ID, Name: row.Name, ExamReady: row.ExamReady, LearningReady: row.LearningReady,
			Preparation: func() *subject.CoursePreparation {
				preparation := subject.ProjectPreparation(subject.PreparationFacts{
					HasMaterials: row.HasMaterials, HasMap: row.HasMap,
					JobSource: row.JobSource, JobStatus: row.JobStatus,
					JobFingerprintCurrent: row.JobFingerprintCurrent,
					GraphExists:           row.GraphExists, GraphSource: row.GraphSource,
					GraphFingerprintCurrent: row.GraphFingerprintCurrent,
					GraphStale:              row.GraphStale,
					MaterialsProcessing:     row.MaterialsProcessing, MaterialsQueued: row.MaterialsQueued,
					MaterialsFailed: row.MaterialsFailed, MaterialsNeedMetadata: row.MaterialsNeedMetadata,
					LearningMapActive: row.LearningMapActive, LearningReady: row.LearningReady,
				})
				return &preparation
			}(),
		})
	}
	return items, nil
}
