package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	materialshttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/transport/http"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
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

type stubOCR struct{}

func (o *stubOCR) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	return "# " + filename + "\n\nФормула: $\\theta_{t+1} = \\theta_t - \\eta \\nabla L$", nil
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	readDeadlines []time.Time
	writeDeadline time.Time
}

func (r *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	r.readDeadlines = append(r.readDeadlines, deadline)
	return nil
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.writeDeadline = deadline
	return nil
}

type deadlineOCR struct {
	t        *testing.T
	recorder *deadlineRecorder
	calls    int
}

func (o *deadlineOCR) Convert(_ context.Context, _ string, _ []byte) (string, error) {
	o.calls++
	if time.Until(o.recorder.writeDeadline) < 119*time.Minute {
		o.t.Fatal("upload handler cuts off the configured two-hour OCR budget")
	}
	return "Материалы лекции", nil
}

func TestUploadQueuesMultipleFilesWithoutRunningOCRInRequest(t *testing.T) {
	recorder := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	ocr := &deadlineOCR{t: t, recorder: recorder}
	svc := application.New(&memoryRepo{}, &memoryS3{}, ocr, time.Now)
	handler := materialshttp.NewHandler(svc, 1024)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, filename := range []string{"first.pdf", "second.pdf"} {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write([]byte("PDF")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/subjects/sub/materials", &body)
	req.SetPathValue("subject_id", "sub")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = httpx.WithPrincipal(req, user.User{ID: "admin", Role: user.Admin})
	handler.UploadMaterials(recorder, req)
	if recorder.Code != http.StatusOK || ocr.calls != 0 {
		t.Fatalf("upload failed: status=%d calls=%d body=%s", recorder.Code, ocr.calls, recorder.Body.String())
	}
	if len(recorder.readDeadlines) < 4 {
		t.Fatal("read deadline was not renewed between files")
	}
	if remaining := time.Until(recorder.writeDeadline); remaining <= 0 || remaining > 30*time.Second {
		t.Fatalf("response deadline was not bounded after processing: %v", remaining)
	}
}

func TestUploadMaterialsCapsBatchAt48Files(t *testing.T) {
	repo := &memoryRepo{}
	svc := application.New(repo, &memoryS3{}, &stubOCR{}, time.Now)
	handler := materialshttp.NewHandler(svc, 1024)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for i := 0; i < materialshttp.MaxFilesPerRequest+1; i++ {
		part, err := writer.CreateFormFile("file", fmt.Sprintf("lecture-%02d.pdf", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("PDF")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/subjects/sub/materials", &body)
	req.SetPathValue("subject_id", "sub")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = httpx.WithPrincipal(req, user.User{ID: "admin", Role: user.Admin})
	recorder := httptest.NewRecorder()
	handler.UploadMaterials(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var result materialshttp.UploadResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Uploaded) != materialshttp.MaxFilesPerRequest || len(result.Failed) != 1 || result.Failed[0].Code != "TOO_MANY_FILES" {
		t.Fatalf("unexpected batch result: uploaded=%d failed=%+v", len(result.Uploaded), result.Failed)
	}
}

func TestHandlerUploadAndList(t *testing.T) {
	repo := &memoryRepo{}
	s3 := &memoryS3{}
	ocr := &stubOCR{}
	svc := application.New(repo, s3, ocr, time.Now)
	handler := materialshttp.NewHandler(svc, 25*1024*1024)

	admin := user.User{ID: "adm-1", Role: user.Admin}
	student := user.User{ID: "stu-1", Role: user.Student}

	makeUploadReq := func() (*http.Request, *httptest.ResponseRecorder) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "lecture1.pdf")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		part.Write([]byte("fake-pdf-content"))
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/admin/subjects/sub-1/materials", &body)
		req.SetPathValue("subject_id", "sub-1")
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return req, httptest.NewRecorder()
	}

	// 1. Unauthenticated request -> 401
	{
		req, rec := makeUploadReq()
		handler.UploadMaterials(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for unauth, got %d", rec.Code)
		}
	}

	// 2. Student request -> 403
	{
		req, rec := makeUploadReq()
		req = httpx.WithPrincipal(req, student)
		handler.UploadMaterials(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for student, got %d", rec.Code)
		}
	}

	// 3. Admin upload -> 200 OK
	var uploadRes materialshttp.UploadResult
	{
		req, rec := makeUploadReq()
		req = httpx.WithPrincipal(req, admin)
		handler.UploadMaterials(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&uploadRes); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(uploadRes.Uploaded) != 1 {
			t.Fatalf("expected 1 uploaded material, got %d", len(uploadRes.Uploaded))
		}
		if uploadRes.Uploaded[0].Status != material.StatusPending || uploadRes.Uploaded[0].ParsedText != "" {
			t.Errorf("expected pending material before background OCR, got status=%s text=%q", uploadRes.Uploaded[0].Status, uploadRes.Uploaded[0].ParsedText)
		}
	}

	// 4. List as student -> 403
	{
		reqList := httptest.NewRequest(http.MethodGet, "/admin/subjects/sub-1/materials", nil)
		reqList.SetPathValue("subject_id", "sub-1")
		reqList = httpx.WithPrincipal(reqList, student)
		recList := httptest.NewRecorder()
		handler.ListMaterials(recList, reqList)
		if recList.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for student list, got %d", recList.Code)
		}
	}

	// 5. List as admin -> 200 OK
	{
		reqList := httptest.NewRequest(http.MethodGet, "/admin/subjects/sub-1/materials", nil)
		reqList.SetPathValue("subject_id", "sub-1")
		reqList = httpx.WithPrincipal(reqList, admin)
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
	}

	// 6. Download raw as admin -> 200 OK with safe headers
	{
		reqRaw := httptest.NewRequest(http.MethodGet, "/admin/subjects/sub-1/materials/"+uploadRes.Uploaded[0].ID+"/raw", nil)
		reqRaw.SetPathValue("subject_id", "sub-1")
		reqRaw.SetPathValue("material_id", uploadRes.Uploaded[0].ID)
		reqRaw = httpx.WithPrincipal(reqRaw, admin)
		recRaw := httptest.NewRecorder()
		handler.DownloadRaw(recRaw, reqRaw)
		if recRaw.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on download raw, got %d", recRaw.Code)
		}
		if !strings.Contains(recRaw.Header().Get("Content-Disposition"), "attachment;") {
			t.Errorf("expected attachment Content-Disposition, got %s", recRaw.Header().Get("Content-Disposition"))
		}
		if recRaw.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("expected nosniff header, got %s", recRaw.Header().Get("X-Content-Type-Options"))
		}
	}

	// 7. Delete as admin -> 200 OK
	{
		reqDel := httptest.NewRequest(http.MethodDelete, "/admin/subjects/sub-1/materials/"+uploadRes.Uploaded[0].ID, nil)
		reqDel.SetPathValue("subject_id", "sub-1")
		reqDel.SetPathValue("material_id", uploadRes.Uploaded[0].ID)
		reqDel = httpx.WithPrincipal(reqDel, admin)
		recDel := httptest.NewRecorder()
		handler.DeleteMaterial(recDel, reqDel)
		if recDel.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on delete, got %d", recDel.Code)
		}
	}
}
