package modelapi

import (
	"context"
	"encoding/json"
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
		var request struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
			Messages        []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "gpt-oss-120b" || request.ReasoningEffort != "medium" || len(request.Messages) != 2 {
			t.Errorf("invalid request: %+v, %v", request, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"content": "{\"summary\": \"Вы различаете линейные модели. Для регуляризации повторите различие L1 и L2.\", \"details\": [{\"id\": \"strength:0\", \"text\": \"Вы понимаете линейные модели.\"}, {\"id\": \"gap:0\", \"text\": \"Повторите различия L1 и L2.\"}]}"
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
	if !strings.Contains(summary.Summary, "различаете линейные модели") || len(summary.Details) != 2 || summary.Details[1].ID != "gap:0" || summary.Details[1].Text != "Повторите различия L1 и L2." {
		t.Errorf("unexpected summary: %+v", summary)
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

func TestStructuredFeedbackRejectsTruncatedOrIncompleteJSON(t *testing.T) {
	for _, tc := range []struct{ name, content, finish string }{
		{"truncated", `{"summary":"Итог","details":[]}`, "length"},
		{"missing cards", `{"summary":"Итог"}`, "stop"},
		{"empty summary", `{"summary":" ","details":[]}`, "stop"},
		{"malformed", `{"summary":"Итог","details":[`, "stop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": tc.content}, "finish_reason": tc.finish}}})
			if _, err := parseWrittenFeedback(raw); err == nil {
				t.Fatal("invalid model completion accepted")
			}
		})
	}
}
