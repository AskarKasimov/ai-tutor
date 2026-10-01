package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) Save(ctx context.Context, tr transcription.Transcription) error {
	_, err := r.pool.Exec(ctx, "INSERT INTO transcriptions(id,user_id,text,created_at) VALUES($1,$2,$3,$4)", tr.ID, tr.OwnerID, tr.Text, tr.CreatedAt)
	return err
}
