package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	materialshttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/transport/http"
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
		return material.Material{}, io.EOF
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

type stubOCR struct{}

func (o *stubOCR) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	return "# " + filename + "\n\nФормула: $\\theta_{t+1} = \\theta_t - \\eta \\nabla L$", nil
}

func TestHandlerUploadAndList(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{}
	svc := application.New(repo, s3, ocr, time.Now)
	handler := materialshttp.NewHandler(svc, 25*1024*1024)

	// Create multipart body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "lecture1.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write([]byte("fake-pdf-content"))
	writer.Close()

	// 1. Upload
	req := httptest.NewRequest(http.MethodPost, "/admin/subjects/sub-1/materials", &body)
	req.SetPathValue("subject_id", "sub-1")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.UploadMaterials(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var uploaded []material.Material
	if err := json.NewDecoder(rec.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(uploaded) != 1 {
		t.Fatalf("expected 1 uploaded material, got %d", len(uploaded))
	}
	if !strings.Contains(uploaded[0].ParsedText, "\\nabla L") {
		t.Errorf("expected LaTeX formula in parsed text, got: %s", uploaded[0].ParsedText)
	}

	// 2. List
	reqList := httptest.NewRequest(http.MethodGet, "/admin/subjects/sub-1/materials", nil)
	reqList.SetPathValue("subject_id", "sub-1")
	recList := httptest.NewRecorder()
	handler.ListMaterials(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recList.Code)
	}

	var listed []material.Material
	if err := json.NewDecoder(recList.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 listed material, got %d", len(listed))
	}

	// 3. Delete
	reqDel := httptest.NewRequest(http.MethodDelete, "/admin/subjects/sub-1/materials/"+uploaded[0].ID, nil)
	reqDel.SetPathValue("subject_id", "sub-1")
	reqDel.SetPathValue("material_id", uploaded[0].ID)
	recDel := httptest.NewRecorder()
	handler.DeleteMaterial(recDel, reqDel)
	if recDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d", recDel.Code)
	}
}
