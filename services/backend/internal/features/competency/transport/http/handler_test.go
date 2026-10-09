package competencyhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/application"
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
			request := httptest.NewRequest(http.MethodPost, "/admin/subjects/subject:intro-to-ml/competency-map/import", nil)
			request.SetPathValue("subject_id", "subject:intro-to-ml")
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
