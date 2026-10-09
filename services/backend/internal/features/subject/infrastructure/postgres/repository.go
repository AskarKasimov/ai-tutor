package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct {
	queries *db.Queries
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool)} }

func (r *Repository) Create(ctx context.Context, id, name string, createdAt int64) (subject.Subject, error) {
	created, err := r.queries.CreateSubject(ctx, db.CreateSubjectParams{ID: id, Name: name, CreatedAt: createdAt})
	if err != nil {
		return subject.Subject{}, err
	}
	return subject.Subject{ID: created.ID, Name: created.Name}, nil
}

func (r *Repository) List(ctx context.Context) ([]subject.Subject, error) {
	rows, err := r.queries.ListSubjects(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]subject.Subject, 0, len(rows))
	for _, row := range rows {
		items = append(items, subject.Subject{ID: row.ID, Name: row.Name, Ready: row.Ready})
	}
	return items, nil
}
