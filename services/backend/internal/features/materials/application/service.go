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

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Repository interface {
	ListKinds(context.Context) ([]material.KindOption, error)
	UpdateMetadata(context.Context, string, string, *int32, *material.Kind) (material.Material, error)
	ReplaceMaterial(context.Context, string, string, material.Material) (material.Material, error)
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
	Week        int32
	Kind        material.Kind
	Filename    string
	ContentType string
	Data        []byte
}

func ValidateWeek(week int32) error {
	if week < 1 {
		return fault.Validation("week", "Неделя должна быть положительным числом.")
	}
	return nil
}

func (s *Service) MaterialKinds(ctx context.Context, actor user.User) ([]material.KindOption, error) {
	if err := s.Authorize(actor); err != nil {
		return nil, err
	}
	kinds, err := s.repo.ListKinds(ctx)
	if err != nil {
		return nil, fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось загрузить виды материалов.")
	}
	return kinds, nil
}

func (s *Service) ValidateMetadata(ctx context.Context, week int32, kind material.Kind) error {
	if err := ValidateWeek(week); err != nil {
		return err
	}
	kinds, err := s.repo.ListKinds(ctx)
	if err != nil {
		return fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось проверить вид материала.")
	}
	for _, option := range kinds {
		if option.Code == kind {
			return nil
		}
	}
	return fault.Validation("kind", "Выберите доступный вид материала.")
}

func ValidatePDF(filename string, data []byte) error {
	if !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		return fault.Validation("file", "Допускаются только файлы PDF.")
	}
	header := data
	if len(header) > 1024 {
		header = header[:1024]
	}
	if !strings.Contains(string(header), "%PDF-") {
		return fault.Validation("file", "Файл не имеет сигнатуры PDF.")
	}
	return nil
}

func (s *Service) Upload(ctx context.Context, actor user.User, subjectID string, input UploadInput) (material.Material, error) {
	if err := s.Authorize(actor); err != nil {
		return material.Material{}, err
	}
	if input.Week != 0 || input.Kind != "" {
		if err := s.ValidateMetadata(ctx, input.Week, input.Kind); err != nil {
			return material.Material{}, err
		}
		if err := ValidatePDF(input.Filename, input.Data); err != nil {
			return material.Material{}, err
		}
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
	if input.Week != 0 || input.Kind != "" {
		contentType = "application/pdf"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.s3.Put(ctx, s3RawKey, contentType, input.Data); err != nil {
		log.Printf("Failed to upload raw file to S3 for %s: %v", cleanFilename, err)
		return material.Material{}, fault.New(fault.Unavailable, "S3_UPLOAD_ERROR", "Хранилище недоступно, попробуйте позже.")
	}

	// OCR and translation run in a durable background job so a long batch does
	// not keep the browser and reverse proxy waiting for the entire pipeline.
	matRecord := material.Material{
		ID:           materialID,
		SubjectID:    subjectID,
		Filename:     cleanFilename,
		ContentType:  contentType,
		FileSize:     int64(len(input.Data)),
		S3RawKey:     s3RawKey,
		S3ParsedKey:  s3ParsedKey,
		ParsedText:   "",
		CharCount:    0,
		Status:       material.StatusPending,
		ErrorMessage: nil,
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}

	if input.Week != 0 || input.Kind != "" {
		matRecord.Week = &input.Week
		matRecord.Kind = &input.Kind
	}
	created, err := s.repo.Insert(ctx, matRecord)
	if err != nil {
		log.Printf("Failed to save material %s to database: %v", cleanFilename, err)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if delErr := s.s3.Delete(cleanupCtx, s3RawKey); delErr != nil {
			log.Printf("Failed to cleanup raw S3 object %s after DB error: %v", s3RawKey, delErr)
		}
		return material.Material{}, fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось сохранить материал в базу данных, попробуйте позже.")
	}

	return created, nil
}

type ReplaceInput struct {
	Filename string
	Data     []byte
}

func (s *Service) Replace(ctx context.Context, actor user.User, subjectID, materialID string, input ReplaceInput) (material.Material, error) {
	current, err := s.Get(ctx, actor, subjectID, materialID)
	if err != nil {
		return material.Material{}, err
	}
	filename := filepath.Base(input.Filename)
	if err := ValidatePDF(filename, input.Data); err != nil {
		return material.Material{}, err
	}
	token := generateID("replace_")
	rawKey := fmt.Sprintf("subjects/%s/raw/%s_%s_%s", subjectID, materialID, token, filename)
	parsedKey := fmt.Sprintf("subjects/%s/parsed/%s_%s_%s.md", subjectID, materialID, token, filename)
	if err = s.s3.Put(ctx, rawKey, "application/pdf", input.Data); err != nil {
		return material.Material{}, fault.New(fault.Unavailable, "S3_UPLOAD_ERROR", "Не удалось сохранить новый PDF.")
	}
	cleanup := func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.s3.Delete(cctx, rawKey)
		_ = s.s3.Delete(cctx, parsedKey)
	}
	updated := current
	updated.Filename = filename
	updated.ContentType = "application/pdf"
	updated.FileSize = int64(len(input.Data))
	updated.S3RawKey = rawKey
	updated.S3ParsedKey = parsedKey
	updated.ParsedText = ""
	updated.S3OCRKey = ""
	updated.OCRReport = nil
	updated.Cleaning = nil
	updated.Processing = &material.Processing{Stage: "queued", Status: "pending", UpdatedAt: s.now().Unix()}
	updated.CharCount = 0
	updated.Status = material.StatusPending
	updated.ErrorMessage = nil
	updated.UpdatedAt = s.now().Unix()
	result, err := s.repo.ReplaceMaterial(ctx, subjectID, materialID, updated)
	if err != nil {
		cleanup()
		if errors.Is(err, material.ErrNotFound) {
			return material.Material{}, fault.New(fault.NotFound, "NOT_FOUND", "Материал не найден.")
		}
		return material.Material{}, fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось заменить материал.")
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	keys := []string{current.S3RawKey, current.S3ParsedKey, current.S3OCRKey}
	if current.Cleaning != nil {
		keys = append(keys, current.Cleaning.S3CleanedKey, current.Cleaning.S3ReportKey)
	}
	for _, key := range keys {
		if key != "" {
			if err := s.s3.Delete(cctx, key); err != nil {
				log.Printf("Failed to clean replaced material object for %s: %v", materialID, err)
			}
		}
	}
	return result, nil
}

func (s *Service) UpdateMetadata(ctx context.Context, actor user.User, subjectID, materialID string, week *int32, kind *material.Kind) (material.Material, error) {
	if err := s.Authorize(actor); err != nil {
		return material.Material{}, err
	}
	if week == nil && kind == nil {
		return material.Material{}, fault.Validation("body", "Укажите week и/или kind.")
	}
	if _, err := s.Get(ctx, actor, subjectID, materialID); err != nil {
		return material.Material{}, err
	}
	if week != nil && kind != nil {
		if err := s.ValidateMetadata(ctx, *week, *kind); err != nil {
			return material.Material{}, err
		}
	} else if week != nil {
		if err := ValidateWeek(*week); err != nil {
			return material.Material{}, err
		}
	} else if kind != nil {
		if err := s.ValidateMetadata(ctx, 1, *kind); err != nil {
			return material.Material{}, err
		}
	}
	updated, err := s.repo.UpdateMetadata(ctx, subjectID, materialID, week, kind)
	if err != nil {
		if errors.Is(err, material.ErrNotFound) {
			return material.Material{}, fault.New(fault.NotFound, "NOT_FOUND", "Материал не найден.")
		}
		return material.Material{}, fault.New(fault.Unavailable, "DATABASE_ERROR", "Не удалось обновить материал.")
	}
	return updated, nil
}

func (s *Service) List(ctx context.Context, actor user.User, subjectID string) ([]material.Material, error) {
	if err := s.Authorize(actor); err != nil {
		return nil, err
	}
	items, err := s.repo.List(ctx, subjectID)
	if err != nil {
		return nil, err
	}
	needsModels := false
	for _, item := range items {
		if item.Status == material.StatusPending && item.Processing != nil && item.Processing.Stage == "queued" && strings.HasSuffix(strings.ToLower(item.Filename), ".pdf") {
			needsModels = true
			break
		}
	}
	if needsModels {
		if readiness, ok := s.ocr.(interface{ Ready(context.Context) error }); ok && readiness.Ready(ctx) != nil {
			for i := range items {
				if items[i].Status == material.StatusPending && items[i].Processing != nil && items[i].Processing.Stage == "queued" && strings.HasSuffix(strings.ToLower(items[i].Filename), ".pdf") {
					items[i].Processing.Stage = "waiting_models"
				}
			}
		}
	}
	return items, nil
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
	if mat.S3OCRKey != "" {
		if delErr := s.s3.Delete(cleanupCtx, mat.S3OCRKey); delErr != nil {
			log.Printf("Failed to delete OCR S3 object for material %s: %v", materialID, delErr)
		}
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
