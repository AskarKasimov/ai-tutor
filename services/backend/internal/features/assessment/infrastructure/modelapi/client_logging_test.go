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

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func TestGradeDoesNotQuoteAnUnknownFieldInTheCause(t *testing.T) {
	const invented = "PRIVATE_STUDENT_ANSWER"
	answer, err := json.Marshal(map[string]any{"score": 0, "verdict": "incorrect", "criterion_results": []any{}, "feedback": []string{}, invented: "value"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(answer)}}}})
	}))
	defer server.Close()

	_, err = New(server.Client(), server.URL, "", time.Second).Grade(context.Background(), application.GradingContext{Role: "basic", MaxScore: 1}, "ответ")
	var modelErr *fault.Error
	if !errors.As(err, &modelErr) || modelErr.Cause == nil {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(modelErr.Cause.Error(), invented) {
		t.Fatalf("the cause quotes the model: %q", modelErr.Cause)
	}
}

// A model answer that breaks the format must leave its reason for the logs, and
// that reason must not repeat what the model wrote.
func TestGradeExplainsARejectedAnswerWithoutQuotingIt(t *testing.T) {
	const feedback = "Текст отзыва модели"
	answer, err := json.Marshal(map[string]any{"score": 0, "verdict": "incorrect", "criterion_results": []any{}, "feedback": feedback})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(answer)}}}})
	}))
	defer server.Close()

	_, err = New(server.Client(), server.URL, "", time.Second).Grade(context.Background(), application.GradingContext{Role: "basic", MaxScore: 1}, "ответ")
	var modelErr *fault.Error
	if !errors.As(err, &modelErr) || modelErr.Code != "INVALID_MODEL_RESPONSE" || modelErr.Cause == nil {
		t.Fatalf("error = %#v", err)
	}
	if cause := modelErr.Cause.Error(); !strings.Contains(cause, "feedback") || strings.Contains(cause, feedback) {
		t.Fatalf("cause = %q", cause)
	}
}
