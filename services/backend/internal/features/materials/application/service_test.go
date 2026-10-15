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

func (m *memoryRepo) Delete(ctx context.Context, subjectID, id string) error {
	delete(m.items, id)
	return nil
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
