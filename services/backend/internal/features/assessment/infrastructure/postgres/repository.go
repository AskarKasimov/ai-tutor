package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) TextByOwner(ctx context.Context, ownerID, transcriptionID string) (string, error) {
	var text string
	err := r.pool.QueryRow(ctx, "SELECT text FROM transcriptions WHERE id=$1 AND user_id=$2", transcriptionID, ownerID).Scan(&text)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fault.New(fault.NotFound, "TRANSCRIPTION_NOT_FOUND", "Расшифровка не найдена.")
	}
	return text, err
}
