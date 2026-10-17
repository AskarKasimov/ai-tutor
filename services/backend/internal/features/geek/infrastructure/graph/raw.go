package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RawStorage interface {
	Get(context.Context, string) ([]byte, error)
}
type Raw struct {
	pool    *pgxpool.Pool
	storage RawStorage
}

func NewRaw(pool *pgxpool.Pool, storage RawStorage) Raw { return Raw{pool: pool, storage: storage} }

func (r Raw) CurrentRaw(ctx context.Context, id string) (application.RawMaterial, error) {
	row, err := db.New(r.pool).GetRawSubjectMaterial(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RawMaterial{}, nil
	}
	if err != nil {
		return application.RawMaterial{}, fmt.Errorf("read raw material record: %w", err)
	}
	data, err := r.storage.Get(ctx, row.S3RawKey)
	if errors.Is(err, s3.ErrNotFound) { return application.RawMaterial{}, nil }
	if err != nil {
		return application.RawMaterial{}, fmt.Errorf("read raw material object: %w", err)
	}
	sum := sha256.Sum256([]byte(row.S3RawKey))
	return application.RawMaterial{Filename: row.Filename, Revision: hex.EncodeToString(sum[:]), Data: data, Found: true}, nil
}
