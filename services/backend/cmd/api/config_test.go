package main

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestHealthcheckURLUsesConfiguredPortAndLoopback(t *testing.T) {
	cases := map[string]string{
		":18002":        "http://127.0.0.1:18002/health",
		"0.0.0.0:18002": "http://127.0.0.1:18002/health",
		"[::]:18002":    "http://127.0.0.1:18002/health",
		"[::1]:18002":   "http://[::1]:18002/health",
	}
	for address, want := range cases {
		got, err := healthcheckURL(address)
		if err != nil || got != want {
			t.Errorf("healthcheckURL(%q) = %q, %v; want %q", address, got, err, want)
		}
	}
}

func TestPrepareQuestionsReadsYAMLWithoutDatabaseOrS3Secrets(t *testing.T) {
	t.Setenv("BACKEND_CONFIG", "../../config/dev.yaml")
	t.Setenv("BACKEND_DATABASE_URL", "")
	t.Setenv("BACKEND_S3_ACCESS_KEY", "")
	t.Setenv("BACKEND_S3_SECRET_KEY", "")
	err := prepareQuestions(t.Context(), zap.NewNop(), []string{"missing-graph.json", "output.json"})
	if err == nil || !strings.Contains(err.Error(), "missing-graph.json") {
		t.Fatalf("prepare-questions error = %v; want graph read failure after config load", err)
	}
}
