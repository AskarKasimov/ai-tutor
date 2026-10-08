package modelapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func TestModelAPIClientSynthesizeSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"content": "{\"summary\": \"Студент показал отличный результат (80%). Рекомендуется закрепить регуляризацию.\"}"
					},
					"finish_reason": "stop"
				}
			]
		}`))
	}))
	defer ts.Close()

	client := New(ts.Client(), ts.URL+"/v1", "gpt-oss-120b", 2*time.Second)
	report := application.DeterministicReport{
		DiagnosticScore: 8,
		MaximumScore:    10,
		ScorePercentage: 80,
		Strengths:       []string{"Линейные модели"},
		ConfirmedGaps: []diagnostic.ConfirmedGap{
			{CompetencyName: "Регуляризация", OutcomeName: "L1 vs L2", FailedCriteria: []string{"зануление весов"}},
		},
	}

	summary, err := client.Synthesize(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "отличный результат") {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestModelAPIClientErrors(t *testing.T) {
	t.Run("Server 500 error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		client := New(ts.Client(), ts.URL+"/v1", "gpt-oss-120b", 2*time.Second)
		_, err := client.Synthesize(context.Background(), application.DeterministicReport{})
		var fe *fault.Error
		if err == nil || !errors.As(err, &fe) || fe.Code != "MODEL_FAILED" {
			t.Fatalf("expected MODEL_FAILED error, got: %v", err)
		}
	})

	t.Run("Invalid JSON response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices": [{"message": {"content": "not json"}, "finish_reason": "stop"}]}`))
		}))
		defer ts.Close()

		client := New(ts.Client(), ts.URL+"/v1", "gpt-oss-120b", 2*time.Second)
		_, err := client.Synthesize(context.Background(), application.DeterministicReport{})
		var fe *fault.Error
		if err == nil || !errors.As(err, &fe) || fe.Code != "INVALID_MODEL_RESPONSE" {
			t.Fatalf("expected INVALID_MODEL_RESPONSE, got: %v", err)
		}
	})
}
