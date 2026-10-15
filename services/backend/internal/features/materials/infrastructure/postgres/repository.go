package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) List(ctx context.Context, subjectID string) ([]material.Material, error) {
	q := db.New(r.pool)
	rows, err := q.ListSubjectMaterials(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list subject materials: %w", err)
	}

	result := make([]material.Material, len(rows))
	for i, row := range rows {
		result[i] = toMaterial(row)
	}
	return result, nil
}

func (r *Repository) Get(ctx context.Context, id string) (material.Material, error) {
	q := db.New(r.pool)
	row, err := q.GetSubjectMaterialByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return material.Material{}, material.ErrNotFound
		}
		return material.Material{}, fmt.Errorf("get subject material %s: %w", id, err)
	}
	return toMaterial(row), nil
}

func (r *Repository) Insert(ctx context.Context, m material.Material) (material.Material, error) {
	q := db.New(r.pool)
	row, err := q.InsertSubjectMaterial(ctx, db.InsertSubjectMaterialParams{
		ID:           m.ID,
		SubjectID:    m.SubjectID,
		Filename:     m.Filename,
		ContentType:  m.ContentType,
		FileSize:     m.FileSize,
		S3RawKey:     m.S3RawKey,
		S3ParsedKey:  m.S3ParsedKey,
		ParsedText:   m.ParsedText,
		CharCount:    int32(m.CharCount),
		Status:       string(m.Status),
		ErrorMessage: m.ErrorMessage,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	})
	if err != nil {
		return material.Material{}, fmt.Errorf("insert subject material: %w", err)
	}
	return toMaterial(row), nil
}

func (r *Repository) Delete(ctx context.Context, subjectID, id string) error {
	q := db.New(r.pool)
	err := q.DeleteSubjectMaterial(ctx, db.DeleteSubjectMaterialParams{
		ID:        id,
		SubjectID: subjectID,
	})
	if err != nil {
		return fmt.Errorf("delete subject material: %w", err)
	}
	return nil
}

func toMaterial(row db.SubjectMaterial) material.Material {
	return material.Material{
		ID:           row.ID,
		SubjectID:    row.SubjectID,
		Filename:     row.Filename,
		ContentType:  row.ContentType,
		FileSize:     row.FileSize,
		S3RawKey:     row.S3RawKey,
		S3ParsedKey:  row.S3ParsedKey,
		ParsedText:   row.ParsedText,
		CharCount:    int(row.CharCount),
		Status:       material.Status(row.Status),
		ErrorMessage: row.ErrorMessage,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
