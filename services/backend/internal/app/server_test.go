package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExamRoutesUseTheirOwnNamespace(t *testing.T) {
	f := newFixture(t)
	if got := f.request(http.MethodGet, "/exam/subjects", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("namespaced exam catalog: got %d, want 401", got.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "https://api.example/subjects", nil)
	response := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unscoped catalog route still registered: got %d, want 404", response.Code)
	}
}

func TestHTTPWriteTimeoutAllowsSlowOperationsToFinish(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		voice, s3, ocr, llm, want time.Duration
	}{
		{"transcription with storage", 30 * time.Second, 30 * time.Second, 60 * time.Second, 0, 210 * time.Second},
		{"ocr", time.Second, time.Second, 180 * time.Second, 0, 210 * time.Second},
		{"geek answer", time.Second, time.Second, time.Second, 120 * time.Second, 510 * time.Second},
		{"long geek turn", 120 * time.Second, 10 * time.Second, time.Second, time.Second, 530 * time.Second},
		{"learning STT plus tutor", 120 * time.Second, 30 * time.Second, time.Second, 120 * time.Second, 570 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{VoiceTimeout: tc.voice, S3Timeout: tc.s3, CourseGraphTimeout: tc.ocr, LLMTimeout: tc.llm}
			if got := cfg.HTTPWriteTimeout(); got != tc.want {
				t.Fatalf("write deadline %s, want %s", got, tc.want)
			}
		})
	}
}
