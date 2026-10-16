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
	cleaning, err := q.ListMaterialCleaning(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list cleaning: %w", err)
	}
	byID := make(map[string]material.Cleaning, len(cleaning))
	for _, job := range cleaning {
		byID[job.MaterialID] = toCleaning(job)
	}
	for i := range result {
		if state, ok := byID[result[i].ID]; ok {
			result[i].Cleaning = &state
		}
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
	result := toMaterial(row)
	job, err := q.GetMaterialCleaning(ctx, id)
	if err == nil {
		state := toCleaning(job)
		result.Cleaning = &state
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return material.Material{}, fmt.Errorf("get cleaning: %w", err)
	}
	return result, nil
}

func (r *Repository) Insert(ctx context.Context, m material.Material) (material.Material, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return material.Material{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
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
	result := toMaterial(row)
	if m.Status == material.StatusParsed && m.ParsedText != "" {
		if err := q.EnqueueMaterialCleaning(ctx, db.EnqueueMaterialCleaningParams{MaterialID: m.ID, UpdatedAt: m.UpdatedAt}); err != nil {
			return material.Material{}, fmt.Errorf("enqueue cleaning: %w", err)
		}
		result.Cleaning = &material.Cleaning{Status: "pending", UpdatedAt: m.UpdatedAt}
	}
	if err := tx.Commit(ctx); err != nil {
		return material.Material{}, err
	}
	return result, nil
}

// Lock the material before reading output keys so deletion cannot race completion.
func (r *Repository) Delete(ctx context.Context, subjectID, id string) (material.Material, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return material.Material{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if _, err = q.LockSubjectMaterial(ctx, id); err != nil {
		return material.Material{}, err
	}
	row, err := q.GetSubjectMaterialByID(ctx, id)
	if err != nil {
		return material.Material{}, err
	}
	result := toMaterial(row)
	if result.SubjectID != subjectID {
		return material.Material{}, material.ErrNotFound
	}
	job, err := q.GetMaterialCleaning(ctx, id)
	if err == nil {
		state := toCleaning(job)
		result.Cleaning = &state
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return material.Material{}, err
	}
	if err = q.DeleteSubjectMaterial(ctx, db.DeleteSubjectMaterialParams{ID: id, SubjectID: subjectID}); err != nil {
		return material.Material{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return material.Material{}, err
	}
	return result, nil
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
