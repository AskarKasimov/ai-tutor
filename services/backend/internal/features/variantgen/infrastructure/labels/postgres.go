package labels

import (
	"context"

	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) LearningReadGraphLabels(ctx context.Context, keys []string) ([]db.LearningGraphLabel, error) {
	return db.New(s.pool).LearningReadGraphLabels(ctx, keys)
}

func (s *PostgresStore) SaveGraphLabels(ctx context.Context, labels []db.LearningSaveGraphLabelParams) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	queries := db.New(tx)
	for _, label := range labels {
		if err := queries.LearningSaveGraphLabel(ctx, label); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
