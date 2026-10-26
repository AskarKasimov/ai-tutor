package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct{ queries *db.Queries }

func New(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool)} }

func (r *Repository) TextByOwner(ctx context.Context, ownerID, transcriptionID string) (string, error) {
	var text string
	text, err := r.queries.GetTranscriptionByOwner(ctx, db.GetTranscriptionByOwnerParams{ID: transcriptionID, UserID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fault.New(fault.NotFound, "TRANSCRIPTION_NOT_FOUND", "Расшифровка не найдена.")
	}
	return text, err
}
