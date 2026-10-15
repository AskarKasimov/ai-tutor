package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
)

type Repository interface {
	List(ctx context.Context, subjectID string) ([]material.Material, error)
	Get(ctx context.Context, id string) (material.Material, error)
	Insert(ctx context.Context, m material.Material) (material.Material, error)
	Delete(ctx context.Context, subjectID, id string) error
}

type S3Storage interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
	Delete(ctx context.Context, key string) error
}

type OCRParser interface {
	Convert(ctx context.Context, filename string, data []byte) (string, error)
}

type Service struct {
	repo Repository
	s3   S3Storage
	ocr  OCRParser
	now  func() time.Time
}

func New(repo Repository, s3 S3Storage, ocr OCRParser, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo: repo,
		s3:   s3,
		ocr:  ocr,
		now:  now,
	}
}

type UploadInput struct {
	Filename    string
	ContentType string
	Data        []byte
}

func (s *Service) Upload(ctx context.Context, subjectID string, input UploadInput) (material.Material, error) {
	if strings.TrimSpace(subjectID) == "" {
		return material.Material{}, fmt.Errorf("subject_id is required")
	}
	if len(input.Data) == 0 {
		return material.Material{}, fmt.Errorf("file data cannot be empty")
	}
	cleanFilename := filepath.Base(input.Filename)
	if cleanFilename == "" || cleanFilename == "." {
		cleanFilename = "lecture.pdf"
	}

	materialID := generateID("mat_")
	nowUnix := s.now().Unix()

	s3RawKey := fmt.Sprintf("subjects/%s/raw/%s_%s", subjectID, materialID, cleanFilename)
	s3ParsedKey := fmt.Sprintf("subjects/%s/parsed/%s_%s.md", subjectID, materialID, cleanFilename)

	// 1. Store raw file in S3
	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.s3.Put(ctx, s3RawKey, contentType, input.Data); err != nil {
		return material.Material{}, fmt.Errorf("failed to upload raw file to S3: %w", err)
	}

	// 2. Parse text with Marker OCR (or fallback)
	parsedText, ocrErr := s.ocr.Convert(ctx, cleanFilename, input.Data)
	status := material.StatusParsed
	var errMsg *string
	if ocrErr != nil {
		log.Printf("OCR conversion error for %s: %v", cleanFilename, ocrErr)
		errStr := ocrErr.Error()
		errMsg = &errStr
		if parsedText == "" {
			status = material.StatusFailed
		}
	}

	// 3. Store parsed markdown in S3 if non-empty
	if len(parsedText) > 0 {
		if err := s.s3.Put(ctx, s3ParsedKey, "text/markdown; charset=utf-8", []byte(parsedText)); err != nil {
			log.Printf("Failed to upload parsed markdown to S3 for %s: %v", cleanFilename, err)
		}
	}

	// 4. Save metadata and parsed text in PostgreSQL
	matRecord := material.Material{
		ID:           materialID,
		SubjectID:    subjectID,
		Filename:     cleanFilename,
		ContentType:  contentType,
		FileSize:     int64(len(input.Data)),
		S3RawKey:     s3RawKey,
		S3ParsedKey:  s3ParsedKey,
		ParsedText:   parsedText,
		CharCount:    len([]rune(parsedText)),
		Status:       status,
		ErrorMessage: errMsg,
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}

	created, err := s.repo.Insert(ctx, matRecord)
	if err != nil {
		return material.Material{}, fmt.Errorf("failed to save material to database: %w", err)
	}

	return created, nil
}

func (s *Service) List(ctx context.Context, subjectID string) ([]material.Material, error) {
	return s.repo.List(ctx, subjectID)
}

func (s *Service) Get(ctx context.Context, subjectID, materialID string) (material.Material, error) {
	mat, err := s.repo.Get(ctx, materialID)
	if err != nil {
		return material.Material{}, err
	}
	if mat.SubjectID != subjectID {
		return material.Material{}, fmt.Errorf("material does not belong to subject")
	}
	return mat, nil
}

func (s *Service) Delete(ctx context.Context, subjectID, materialID string) error {
	mat, err := s.Get(ctx, subjectID, materialID)
	if err != nil {
		return err
	}

	_ = s.s3.Delete(ctx, mat.S3RawKey)
	_ = s.s3.Delete(ctx, mat.S3ParsedKey)

	return s.repo.Delete(ctx, subjectID, materialID)
}

func (s *Service) OpenRaw(ctx context.Context, subjectID, materialID string) (io.ReadCloser, string, int64, string, error) {
	mat, err := s.Get(ctx, subjectID, materialID)
	if err != nil {
		return nil, "", 0, "", err
	}

	rc, contentType, size, err := s.s3.Open(ctx, mat.S3RawKey)
	if err != nil {
		return nil, "", 0, "", err
	}
	return rc, contentType, size, mat.Filename, nil
}

func generateID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
