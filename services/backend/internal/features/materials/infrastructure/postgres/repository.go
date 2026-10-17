package postgres

import (
	"context"
	"encoding/json"
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
	ocrRows, err := q.ListMaterialOCRProgress(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list OCR progress: %w", err)
	}
	ocrByID := make(map[string]material.Processing, len(ocrRows))
	for _, job := range ocrRows {
		ocrByID[job.MaterialID] = material.Processing{Stage: ocrStage(job.Status, job.Stage), Status: job.Status, Attempts: int(job.Attempts), UpdatedAt: job.UpdatedAt}
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
		if progress, ok := ocrByID[result[i].ID]; ok {
			result[i].Processing = &progress
		}
		if state, ok := byID[result[i].ID]; ok {
			result[i].Cleaning = &state
			if state.Status == "pending" || state.Status == "processing" {
				result[i].Processing = &material.Processing{Stage: "cleaning", Status: state.Status, Attempts: state.Attempts, UpdatedAt: state.UpdatedAt}
			}
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
	ocrProgress, err := q.GetMaterialOCRProgress(ctx, id)
	if err == nil {
		result.Processing = &material.Processing{Stage: ocrStage(ocrProgress.Status, ocrProgress.Stage), Status: ocrProgress.Status, Attempts: int(ocrProgress.Attempts), UpdatedAt: ocrProgress.UpdatedAt}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return material.Material{}, fmt.Errorf("get OCR progress: %w", err)
	}
	job, err := q.GetMaterialCleaning(ctx, id)
	if err == nil {
		state := toCleaning(job)
		result.Cleaning = &state
		if state.Status == "pending" || state.Status == "processing" {
			result.Processing = &material.Processing{Stage: "cleaning", Status: state.Status, Attempts: state.Attempts, UpdatedAt: state.UpdatedAt}
		}
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
		Week:         m.Week,
		Kind:         kindString(m.Kind),
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
	if m.Status == material.StatusPending {
		if err := q.EnqueueMaterialOCR(ctx, db.EnqueueMaterialOCRParams{MaterialID: m.ID, UpdatedAt: m.UpdatedAt}); err != nil {
			return material.Material{}, fmt.Errorf("enqueue OCR: %w", err)
		}
	} else if m.Status == material.StatusParsed && m.ParsedText != "" {
		if err := q.EnqueueMaterialCleaning(ctx, db.EnqueueMaterialCleaningParams{MaterialID: m.ID, UpdatedAt: m.UpdatedAt}); err != nil {
			return material.Material{}, fmt.Errorf("enqueue cleaning: %w", err)
		}
		result.Cleaning = &material.Cleaning{Status: "pending", UpdatedAt: m.UpdatedAt}
	}
	if m.Status == material.StatusPending {
		result.Processing = &material.Processing{Stage: "queued", Status: "pending", UpdatedAt: m.UpdatedAt}
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
	result := material.Material{
		Week:         row.Week,
		ID:           row.ID,
		SubjectID:    row.SubjectID,
		Filename:     row.Filename,
		ContentType:  row.ContentType,
		FileSize:     row.FileSize,
		S3RawKey:     row.S3RawKey,
		S3ParsedKey:  row.S3ParsedKey,
		S3OCRKey:     row.S3OcrKey,
		ParsedText:   row.ParsedText,
		CharCount:    int(row.CharCount),
		Status:       material.Status(row.Status),
		ErrorMessage: row.ErrorMessage,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if row.Kind != nil {
		kind := material.Kind(*row.Kind)
		result.Kind = &kind
	}
	if len(row.OcrReport) > 0 {
		_ = json.Unmarshal(row.OcrReport, &result.OCRReport)
	}
	return result
}

func ocrStage(status, stage string) string {
	switch status {
	case "processing":
		return stage
	case "failed":
		return "failed"
	case "ready":
		return "ready"
	default:
		return "queued"
	}
}

func (r *Repository) ListKinds(ctx context.Context) ([]material.KindOption, error) {
	rows, err := db.New(r.pool).ListMaterialKinds(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]material.KindOption, 0, len(rows))
	for _, row := range rows {
		result = append(result, material.KindOption{Code: material.Kind(row.Code), Label: row.Label})
	}
	return result, nil
}
func kindString(kind *material.Kind) *string {
	if kind == nil {
		return nil
	}
	value := string(*kind)
	return &value
}
func (r *Repository) UpdateMetadata(ctx context.Context, subjectID, id string, week *int32, kind *material.Kind) (material.Material, error) {
	_, err := db.New(r.pool).UpdateMaterialMetadata(ctx, db.UpdateMaterialMetadataParams{SubjectID: subjectID, ID: id, Week: week, Kind: kindString(kind)})
	if errors.Is(err, pgx.ErrNoRows) {
		return material.Material{}, material.ErrNotFound
	}
	if err != nil {
		return material.Material{}, err
	}
	return r.Get(ctx, id)
}

// Replacing the input cancels existing leases under the same material lock used
// by worker completion. Old OCR/cleaning results cannot publish into the new job.
func (r *Repository) ReplaceMaterial(ctx context.Context, subjectID, id string, m material.Material) (material.Material, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return material.Material{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if _, err = q.LockSubjectMaterial(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return material.Material{}, material.ErrNotFound
	} else if err != nil {
		return material.Material{}, err
	}
	row, err := q.ReplaceSubjectMaterial(ctx, db.ReplaceSubjectMaterialParams{SubjectID: subjectID, ID: id, Filename: m.Filename, ContentType: m.ContentType, FileSize: m.FileSize, S3RawKey: m.S3RawKey, S3ParsedKey: m.S3ParsedKey, UpdatedAt: m.UpdatedAt})
	if errors.Is(err, pgx.ErrNoRows) {
		return material.Material{}, material.ErrNotFound
	}
	if err != nil {
		return material.Material{}, err
	}
	if err = q.DeleteMaterialCleaningJob(ctx, id); err != nil {
		return material.Material{}, err
	}
	if err = q.DeleteMaterialOCRJob(ctx, id); err != nil {
		return material.Material{}, err
	}
	if err = q.EnqueueMaterialOCR(ctx, db.EnqueueMaterialOCRParams{MaterialID: id, UpdatedAt: m.UpdatedAt}); err != nil {
		return material.Material{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return material.Material{}, err
	}
	result := toMaterial(row)
	result.Processing = &material.Processing{Stage: "queued", Status: "pending", UpdatedAt: m.UpdatedAt}
	return result, nil
}
