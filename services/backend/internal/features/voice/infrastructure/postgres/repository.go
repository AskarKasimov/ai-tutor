package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct{ queries *db.Queries }

func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool)} }
func (r *Repository) Save(ctx context.Context, tr transcription.Transcription) error {
	return r.queries.InsertTranscription(ctx, db.InsertTranscriptionParams{ID: tr.ID, UserID: tr.OwnerID, Text: tr.Text, CreatedAt: tr.CreatedAt})
}
