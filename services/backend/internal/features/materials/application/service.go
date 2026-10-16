package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	Delete(ctx context.Context, subjectID, id string) (material.Material, error)
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

type Translator interface {
	Translate(ctx context.Context, markdown string) (string, error)
}

type Service struct {
	repo       Repository
	s3         S3Storage
	ocr        OCRParser
	translator Translator
	now        func() time.Time
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

func (s *Service) WithTranslator(translator Translator) *Service {
	s.translator = translator
	return s
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
	s3TranslatedKey := fmt.Sprintf("subjects/%s/translated/%s_%s.md", subjectID, materialID, cleanFilename)

	// 1. Store raw file in S3
	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.s3.Put(ctx, s3RawKey, contentType, input.Data); err != nil {
		log.Printf("Failed to upload raw file to S3 for %s: %v", cleanFilename, err)
		return material.Material{}, fault.New(fault.Unavailable, "S3_UPLOAD_ERROR", "Хранилище недоступно, попробуйте позже.")
	}

	// 2. Parse text with course-graph OCR
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
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			_ = s.s3.Delete(cleanupCtx, s3RawKey)
			cancel()
			return material.Material{}, fault.New(fault.Unavailable, "S3_UPLOAD_ERROR", "Не удалось сохранить извлечённый текст, попробуйте позже.")
		}
	}

	// 4. Translate if needed and translator configured
	finalText := parsedText
	targetS3ParsedKey := s3ParsedKey
	if len(parsedText) > 0 && s.translator != nil {
		translatedText, transErr := s.translator.Translate(ctx, parsedText)
		if transErr != nil {
			log.Printf("Translation warning for %s: %v (preserving original OCR text)", cleanFilename, transErr)
			status = material.StatusFailed
			message := "Перевод не завершён; исходный OCR сохранён: " + transErr.Error()
			errMsg = &message
		} else if translatedText != "" && translatedText != parsedText {
			if err := s.s3.Put(ctx, s3TranslatedKey, "text/markdown; charset=utf-8", []byte(translatedText)); err != nil {
				log.Printf("Failed to upload translated markdown to S3 for %s: %v", cleanFilename, err)
				status = material.StatusFailed
				message := "Не удалось сохранить перевод; исходный OCR сохранён."
				errMsg = &message
			} else {
				targetS3ParsedKey = s3TranslatedKey
				finalText = translatedText
			}
		}
	}

	// 5. Save metadata and parsed text in PostgreSQL
	matRecord := material.Material{
		ID:           materialID,
		SubjectID:    subjectID,
		Filename:     cleanFilename,
		ContentType:  contentType,
		FileSize:     int64(len(input.Data)),
		S3RawKey:     s3RawKey,
		S3ParsedKey:  targetS3ParsedKey,
		ParsedText:   finalText,
		CharCount:    utf8.RuneCountInString(finalText),
		Status:       status,
		ErrorMessage: errMsg,
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}

	created, err := s.repo.Insert(ctx, matRecord)
	if err != nil {
		log.Printf("Failed to save material %s to database: %v", cleanFilename, err)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if delErr := s.s3.Delete(cleanupCtx, s3RawKey); delErr != nil {
			log.Printf("Failed to cleanup raw S3 object %s after DB error: %v", s3RawKey, delErr)
		}
		if delErr := s.s3.Delete(cleanupCtx, s3ParsedKey); delErr != nil {
			log.Printf("Failed to cleanup parsed S3 object %s after DB error: %v", s3ParsedKey, delErr)
		}
		if targetS3ParsedKey == s3TranslatedKey {
			_ = s.s3.Delete(cleanupCtx, s3TranslatedKey)
		}
		return material.Material{}, fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось сохранить материал в базу данных, попробуйте позже.")
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
		if errors.Is(err, material.ErrNotFound) {
			return material.Material{}, fault.New(fault.NotFound, "NOT_FOUND", "Материал не найден.")
		}
		log.Printf("Failed to get material %s from repository: %v", materialID, err)
		return material.Material{}, fault.New(fault.Unavailable, "DATABASE_ERROR", "База данных временно недоступна, попробуйте позже.")
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

	deleted, err := s.repo.Delete(ctx, subjectID, materialID)
	if err != nil {
		log.Printf("Failed to delete material %s from repository: %v", materialID, err)
		return fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось удалить материал из базы данных.")
	}

	mat = deleted
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if delErr := s.s3.Delete(cleanupCtx, mat.S3RawKey); delErr != nil {
		log.Printf("Failed to delete raw S3 object %s for material %s: %v", mat.S3RawKey, materialID, delErr)
	}
	if delErr := s.s3.Delete(cleanupCtx, mat.S3ParsedKey); delErr != nil {
		log.Printf("Failed to delete parsed S3 object %s for material %s: %v", mat.S3ParsedKey, materialID, delErr)
	}
	parsedKey := fmt.Sprintf("subjects/%s/parsed/%s_%s.md", mat.SubjectID, mat.ID, mat.Filename)
	if parsedKey != mat.S3ParsedKey {
		_ = s.s3.Delete(cleanupCtx, parsedKey)
	}
	translatedKey := fmt.Sprintf("subjects/%s/translated/%s_%s.md", mat.SubjectID, mat.ID, mat.Filename)
	if translatedKey != mat.S3ParsedKey {
		_ = s.s3.Delete(cleanupCtx, translatedKey)
	}
	if mat.Cleaning != nil {
		for _, key := range []string{mat.Cleaning.S3CleanedKey, mat.Cleaning.S3ReportKey} {
			if key != "" {
				if err := s.s3.Delete(cleanupCtx, key); err != nil {
					log.Printf("Failed to delete cleaning object for %s: %v", materialID, err)
				}
			}
		}
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
