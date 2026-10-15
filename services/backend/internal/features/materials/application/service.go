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
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
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

// Authorize ensures only admin users can access materials operations.
func (s *Service) Authorize(actor user.User) error {
	if actor.Role != user.Admin {
		return fault.New(fault.Forbidden, "FORBIDDEN", "Операция доступна только admin.")
	}
	return nil
}

type UploadInput struct {
	Filename    string
	ContentType string
	Data        []byte
}

func (s *Service) Upload(ctx context.Context, actor user.User, subjectID string, input UploadInput) (material.Material, error) {
	if err := s.Authorize(actor); err != nil {
		return material.Material{}, err
	}
	if strings.TrimSpace(subjectID) == "" {
		return material.Material{}, fault.New(fault.Invalid, "BAD_REQUEST", "ID предмета обязателен.")
	}
	if len(input.Data) == 0 {
		return material.Material{}, fault.New(fault.Invalid, "BAD_REQUEST", "Файл не может быть пустым.")
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
		return material.Material{}, fault.New(fault.Unavailable, "S3_UPLOAD_ERROR", fmt.Sprintf("failed to upload raw file to S3: %v", err))
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
		CharCount:    utf8.RuneCountInString(parsedText),
		Status:       status,
		ErrorMessage: errMsg,
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}

	created, err := s.repo.Insert(ctx, matRecord)
	if err != nil {
		_ = s.s3.Delete(ctx, s3RawKey)
		_ = s.s3.Delete(ctx, s3ParsedKey)
		return material.Material{}, fault.New(fault.Unavailable, "DATABASE_ERROR", fmt.Sprintf("failed to save material to database: %v", err))
	}

	return created, nil
}

func (s *Service) List(ctx context.Context, actor user.User, subjectID string) ([]material.Material, error) {
	if err := s.Authorize(actor); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, subjectID)
}

func (s *Service) Get(ctx context.Context, actor user.User, subjectID, materialID string) (material.Material, error) {
	if err := s.Authorize(actor); err != nil {
		return material.Material{}, err
	}
	mat, err := s.repo.Get(ctx, materialID)
	if err != nil {
		return material.Material{}, fault.New(fault.NotFound, "NOT_FOUND", "Материал не найден.")
	}
	if mat.SubjectID != subjectID {
		return material.Material{}, fault.New(fault.NotFound, "NOT_FOUND", "Материал не принадлежит указанному предмету.")
	}
	return mat, nil
}

func (s *Service) Delete(ctx context.Context, actor user.User, subjectID, materialID string) error {
	if err := s.Authorize(actor); err != nil {
		return err
	}
	mat, err := s.Get(ctx, actor, subjectID, materialID)
	if err != nil {
		return err
	}

	_ = s.s3.Delete(ctx, mat.S3RawKey)
	_ = s.s3.Delete(ctx, mat.S3ParsedKey)

	if err := s.repo.Delete(ctx, subjectID, materialID); err != nil {
		return fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось удалить материал из базы данных.")
	}
	return nil
}

func (s *Service) OpenRaw(ctx context.Context, actor user.User, subjectID, materialID string) (io.ReadCloser, string, int64, string, error) {
	if err := s.Authorize(actor); err != nil {
		return nil, "", 0, "", err
	}
	mat, err := s.Get(ctx, actor, subjectID, materialID)
	if err != nil {
		return nil, "", 0, "", err
	}

	rc, contentType, size, err := s.s3.Open(ctx, mat.S3RawKey)
	if err != nil {
		return nil, "", 0, "", fault.New(fault.NotFound, "NOT_FOUND", "Исходный файл не найден в хранилище.")
	}
	return rc, contentType, size, mat.Filename, nil
}

func generateID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
