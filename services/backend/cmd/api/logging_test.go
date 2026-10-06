package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

func TestProductionLoggerPreservesEveryRequestInBurst(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			path := t.TempDir() + "/requests.jsonl"
			cfg := loggerConfig()
			cfg.OutputPaths = []string{path}
			logger, err := cfg.Build()
			if err != nil {
				t.Fatal(err)
			}
			defer logger.Sync()
			handler := httpx.RequestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			for range 500 {
				handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))
			}
			if err := logger.Sync(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if count := strings.Count(string(data), "\n"); count != 500 {
				t.Fatalf("500 completed requests produced %d log entries", count)
			}
		})
	}
}
