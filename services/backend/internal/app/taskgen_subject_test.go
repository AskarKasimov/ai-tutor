package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTaskgenEndpointsRequireNonblankSubjectID(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	for _, tc := range []struct {
		name, path, body string
		idempotency      bool
	}{
		{"generate missing", "/tasks/generate", `{"outcome_id":"outcome-1"}`, true},
		{"generate blank", "/tasks/generate", `{"subject_id":"  ","outcome_id":"outcome-1"}`, true},
		{"materials missing", "/admin/materials", `{"name":"Notes","content":"Text","outcome_ids":["outcome-1"]}`, false},
		{"materials blank", "/admin/materials", `{"subject_id":"  ","name":"Notes","content":"Text","outcome_ids":["outcome-1"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://api.example"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.idempotency {
				req.Header.Set("Idempotency-Key", "missing-subject")
			}
			req.AddCookie(admin)
			response := httptest.NewRecorder()
			f.handler.ServeHTTP(response, req)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
