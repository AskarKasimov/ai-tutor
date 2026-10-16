package application_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
)

type memoryRepo struct {
	items map[string]material.Material
}

func (m *memoryRepo) List(ctx context.Context, subjectID string) ([]material.Material, error) {
	var res []material.Material
	for _, item := range m.items {
		if item.SubjectID == subjectID {
			res = append(res, item)
		}
	}
	return res, nil
}

func (m *memoryRepo) Get(ctx context.Context, id string) (material.Material, error) {
	item, ok := m.items[id]
	if !ok {
		return material.Material{}, material.ErrNotFound
	}
	return item, nil
}

func (m *memoryRepo) Insert(ctx context.Context, mat material.Material) (material.Material, error) {
	if m.items == nil {
		m.items = make(map[string]material.Material)
	}
	m.items[mat.ID] = mat
	return mat, nil
}

func (m *memoryRepo) Delete(ctx context.Context, subjectID, id string) (material.Material, error) {
	item := m.items[id]
	delete(m.items, id)
	return item, nil
}

type memoryS3 struct {
	objects map[string][]byte
}

func (s *memoryS3) Put(ctx context.Context, key, contentType string, data []byte) error {
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[key] = data
	return nil
}

func (s *memoryS3) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, io.EOF
	}
	return data, nil
}

func (s *memoryS3) Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, "", 0, io.EOF
	}
	return io.NopCloser(strings.NewReader(string(data))), "application/pdf", int64(len(data)), nil
}

func (s *memoryS3) Delete(ctx context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

type stubOCR struct {
	markdown string
	err      error
}

func (o *stubOCR) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	if o.err != nil {
		return "", o.err
	}
	if o.markdown != "" {
		return o.markdown, nil
	}
	return "## " + filename + "\n\nФормула: $y = Wx + b$", nil
}

func TestUploadMaterial(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Лекция 1\n\nТест $E = mc^2$"}
	fixedTime := time.Unix(1700000000, 0)
	admin := user.User{ID: "usr-admin", Role: user.Admin}
	student := user.User{ID: "usr-student", Role: user.Student}

	svc := application.New(repo, s3, ocr, func() time.Time { return fixedTime })

	// Test student authorization failure
	_, err := svc.Upload(context.Background(), student, "sub-123", application.UploadInput{
		Filename:    "lecture-1.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-fake-content"),
	})
	if err == nil {
		t.Fatalf("expected error for student actor on upload, got nil")
	}

	mat, err := svc.Upload(context.Background(), admin, "sub-123", application.UploadInput{
		Filename:    "lecture-1.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-fake-content"),
	})
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	if mat.SubjectID != "sub-123" {
		t.Errorf("expected subject_id sub-123, got %s", mat.SubjectID)
	}
	if mat.Filename != "lecture-1.pdf" {
		t.Errorf("expected filename lecture-1.pdf, got %s", mat.Filename)
	}
	if mat.Status != material.StatusParsed {
		t.Errorf("expected status parsed, got %s", mat.Status)
	}
	if !strings.Contains(mat.ParsedText, "$E = mc^2$") {
		t.Errorf("expected formula in parsed_text, got %s", mat.ParsedText)
	}
	if len(s3.objects) != 2 {
		t.Errorf("expected 2 objects in S3 (raw and parsed), got %d", len(s3.objects))
	}

	// Verify listing by student fails
	if _, err := svc.List(context.Background(), student, "sub-123"); err == nil {
		t.Fatalf("expected error for student actor on list, got nil")
	}

	// Verify listing by admin
	list, err := svc.List(context.Background(), admin, "sub-123")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 listed item, got %d (err: %v)", len(list), err)
	}

	// Verify delete by student fails
	if err := svc.Delete(context.Background(), student, "sub-123", mat.ID); err == nil {
		t.Fatalf("expected error for student actor on delete, got nil")
	}

	// Verify delete by admin
	err = svc.Delete(context.Background(), admin, "sub-123", mat.ID)
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if len(s3.objects) != 0 {
		t.Errorf("expected S3 objects to be deleted, remaining: %d", len(s3.objects))
	}
}

type failingParsedS3 struct{ memoryS3 }

func (s *failingParsedS3) Put(ctx context.Context, key, contentType string, data []byte) error {
	if strings.Contains(key, "/parsed/") {
		return io.ErrUnexpectedEOF
	}
	return s.memoryS3.Put(ctx, key, contentType, data)
}

func TestFailedParsedStorageDoesNotCreateCleaningTask(t *testing.T) {
	repo := &memoryRepo{}
	storage := &failingParsedS3{}
	svc := application.New(repo, storage, &stubOCR{}, nil)
	_, err := svc.Upload(context.Background(), user.User{Role: user.Admin}, "sub", application.UploadInput{Filename: "lecture.md", Data: []byte("Теория")})
	if err == nil {
		t.Fatal("parsed storage failure ignored")
	}
	if len(repo.items) != 0 || len(storage.objects) != 0 {
		t.Fatal("failed upload left a material or raw object")
	}
}

type stubTranslator struct {
	translateFn func(ctx context.Context, markdown string) (string, error)
	calls       int
}

func (st *stubTranslator) Translate(ctx context.Context, markdown string) (string, error) {
	st.calls++
	if st.translateFn != nil {
		return st.translateFn(ctx, markdown)
	}
	return "Перевод: " + markdown, nil
}

func TestUploadMaterialWithTranslation(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Lecture 1\n\nBreadth-first search algorithm."}
	fixedTime := time.Unix(1700000000, 0)
	admin := user.User{ID: "usr-admin", Role: user.Admin}

	trans := &stubTranslator{
		translateFn: func(ctx context.Context, markdown string) (string, error) {
			return "# Лекция 1\n\nАлгоритм поиска в ширину.", nil
		},
	}

	svc := application.New(repo, s3, ocr, func() time.Time { return fixedTime }).WithTranslator(trans)

	mat, err := svc.Upload(context.Background(), admin, "sub-cs", application.UploadInput{
		Filename:    "ai-lecture.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-dummy"),
	})
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	if trans.calls != 1 {
		t.Errorf("expected 1 call to translator, got %d", trans.calls)
	}

	if !strings.Contains(mat.ParsedText, "Алгоритм поиска в ширину") {
		t.Errorf("expected translated Russian text, got: %s", mat.ParsedText)
	}

	if !strings.Contains(mat.S3ParsedKey, "/translated/") {
		t.Errorf("expected S3ParsedKey to point to translated object, got: %s", mat.S3ParsedKey)
	}

	// Verify S3 objects: raw, parsed (original), translated
	if len(s3.objects) != 3 {
		t.Errorf("expected 3 objects in S3 (raw, parsed, translated), got %d: %v", len(s3.objects), s3.objects)
	}

	// Verify Delete cleans up all 3 S3 objects
	err = svc.Delete(context.Background(), admin, "sub-cs", mat.ID)
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if len(s3.objects) != 0 {
		t.Errorf("expected all 3 S3 objects to be deleted, remaining: %d (%v)", len(s3.objects), s3.objects)
	}
}

func TestUploadMaterialTranslationFailureFallback(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{markdown: "# Lecture 1\n\nOriginal English text."}
	fixedTime := time.Unix(1700000000, 0)
	admin := user.User{ID: "usr-admin", Role: user.Admin}

	trans := &stubTranslator{
		translateFn: func(ctx context.Context, markdown string) (string, error) {
			return "", io.ErrUnexpectedEOF
		},
	}

	svc := application.New(repo, s3, ocr, func() time.Time { return fixedTime }).WithTranslator(trans)

	mat, err := svc.Upload(context.Background(), admin, "sub-cs", application.UploadInput{
		Filename:    "ai-lecture.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-dummy"),
	})
	if err != nil {
		t.Fatalf("upload should succeed even when translation fails: %v", err)
	}

	// Should fallback to original OCR text
	if mat.ParsedText != "# Lecture 1\n\nOriginal English text." {
		t.Errorf("expected fallback to original English text, got: %s", mat.ParsedText)
	}

	if !strings.Contains(mat.S3ParsedKey, "/parsed/") {
		t.Errorf("expected S3ParsedKey to point to parsed object on fallback, got: %s", mat.S3ParsedKey)
	}

	if mat.Status != material.StatusParsed {
		t.Errorf("expected status parsed, got: %s", mat.Status)
	}
}
