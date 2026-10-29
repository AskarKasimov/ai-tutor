package competencyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type unreadableBody struct{}

func (unreadableBody) Read([]byte) (int, error) { panic("unauthorized body was read") }
func (unreadableBody) Close() error             { return nil }

func TestImportChecksAuthorizationBeforeReadingBody(t *testing.T) {
	handler := New(application.New(nil, nil, time.Now), 25<<20)
	for _, tc := range []struct {
		name          string
		authenticated bool
		status        int
		code          string
	}{
		{"missing principal", false, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"student", true, http.StatusForbidden, "FORBIDDEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/admin/subjects/subject:test/competency-map/import", nil)
			request.SetPathValue("subject_id", "subject:test")
			request.Body = unreadableBody{}
			if tc.authenticated {
				request = httpx.WithPrincipal(request, user.User{ID: "student", Role: user.Student})
			}
			response := httptest.NewRecorder()
			handler.ImportSubject(response, request)
			var failure struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || failure.Code != tc.code {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type testParser struct{}

func (testParser) Parse([]byte) (competencymap.Map, error) { return competencymap.Map{}, nil }

type conflictRepository struct{ revision *int64 }

func (r *conflictRepository) Read(context.Context, string) (competencymap.Snapshot, error) {
	return competencymap.Snapshot{}, nil
}
func (r *conflictRepository) Replace(_ context.Context, _, _ string, _ competencymap.Map, _ int64, revision *int64) (competencymap.ImportResult, error) {
	r.revision = revision
	return competencymap.ImportResult{}, fault.New(fault.Conflict, "COMPETENCY_MAP_CHANGED", "Карта изменилась.")
}
func TestImportRevisionHeaderAndConflict(t *testing.T) {
	repo := &conflictRepository{}
	handler := New(application.New(repo, testParser{}, time.Now), 25<<20)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="map.csv"`)
	header.Set("Content-Type", "text/csv")
	part, err := form.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("map")); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("X-Expected-Map-Revision", "7")
	request.SetPathValue("subject_id", "subject:test")
	request = httpx.WithPrincipal(request, user.User{ID: "admin", Role: user.Admin})
	response := httptest.NewRecorder()
	handler.ImportSubject(response, request)
	if response.Code != http.StatusConflict || repo.revision == nil || *repo.revision != 7 {
		t.Fatalf("status=%d revision=%v body=%s", response.Code, repo.revision, response.Body.String())
	}
	var failure struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != "COMPETENCY_MAP_CHANGED" {
		t.Fatalf("body=%s", response.Body.String())
	}
}
