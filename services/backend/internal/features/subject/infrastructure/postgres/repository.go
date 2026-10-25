package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct {
	queries *db.Queries
	mode    string
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool), mode: "exam"} }

func NewLearning(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: db.New(pool), mode: "learn"}
}

func (r *Repository) Create(ctx context.Context, id, name string, createdAt int64) (subject.Subject, error) {
	var createdID, createdName string
	var err error
	if r.mode == "learn" {
		created, createErr := r.queries.CreateLearningSubject(ctx, db.CreateLearningSubjectParams{ID: id, Name: name, CreatedAt: createdAt})
		err = createErr
		createdID, createdName = created.ID, created.Name
	} else {
		created, createErr := r.queries.CreateSubject(ctx, db.CreateSubjectParams{ID: id, Name: name, CreatedAt: createdAt})
		err = createErr
		createdID, createdName = created.ID, created.Name
	}
	if err != nil {
		return subject.Subject{}, err
	}
	return subject.Subject{ID: createdID, Name: createdName}, nil
}

func (r *Repository) List(ctx context.Context) ([]subject.Subject, error) {
	items := make([]subject.Subject, 0)
	if r.mode == "learn" {
		rows, err := r.queries.ListLearningSubjects(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			items = append(items, subject.Subject{ID: row.ID, Name: row.Name, Ready: row.Ready})
		}
	} else {
		rows, err := r.queries.ListSubjects(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			items = append(items, subject.Subject{ID: row.ID, Name: row.Name, Ready: row.Ready})
		}
	}
	return items, nil
}
