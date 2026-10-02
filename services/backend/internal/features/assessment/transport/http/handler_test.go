package assessmenthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type transcriptionStub struct{ owner string }

func (s *transcriptionStub) TextByOwner(_ context.Context, owner, _ string) (string, error) {
	s.owner = owner
	return "Классификация, потому что два класса", nil
}

type graderStub struct{}

func (graderStub) Grade(_ context.Context, _ application.Task, _ string) (application.Evaluation, error) {
	return application.Evaluation{Score: 2, Feedback: []string{"Верно.", "Два класса.", "Закрепите тему."}}, nil
}

func TestEvaluateRequiresAuthAndReturnsGrade(t *testing.T) {
	repo := &transcriptionStub{}
	handler := New(application.New(repo, graderStub{}))
	requestBody := `{"transcription_id":"tr-1","question":"Что прогнозирует банк?","options":["Классификация","Регрессия"],"voice_instruction":"Назовите тип","correct_answer":"Классификация"}`
	unauthorized := httptest.NewRecorder()
	handler.Evaluate(unauthorized, httptest.NewRequest(http.MethodPost, "/assessments/evaluate", strings.NewReader(requestBody)))
	if unauthorized.Code != http.StatusUnauthorized || repo.owner != "" {
		t.Fatalf("unauthorized request: %d, owner=%q", unauthorized.Code, repo.owner)
	}
	request := httptest.NewRequest(http.MethodPost, "/assessments/evaluate", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request = httpx.WithPrincipal(request, user.User{ID: "student-1", Role: user.Student})
	response := httptest.NewRecorder()
	handler.Evaluate(response, request)
	var result EvaluateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || repo.owner != "student-1" || result.Score != 2 || len(result.Feedback) != 3 {
		t.Fatalf("response=%d %s, owner=%q", response.Code, response.Body.String(), repo.owner)
	}
}
