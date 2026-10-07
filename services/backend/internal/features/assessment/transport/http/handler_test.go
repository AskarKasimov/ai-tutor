package assessmenthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type transcriptionStub struct{ owner string }

func (s *transcriptionStub) TextByOwner(_ context.Context, owner, _ string) (string, error) {
	s.owner = owner
	return "Классификация, потому что два класса", nil
}

type contextStub struct{ taskID string }

func (s *contextStub) ContextForTask(_ context.Context, taskID string) (application.GradingContext, error) {
	s.taskID = taskID
	return application.GradingContext{
		TaskID: taskID, Question: "Вопрос", ReferenceAnswer: "Классификация",
		Criteria: []application.Criterion{
			{Key: "task_type", Description: "Назван тип задачи"},
			{Key: "justification", Description: "Есть объяснение"},
		},
		MaterialContext: application.MaterialContext{Knowledge: "K", Skills: "S"},
	}, nil
}

type graderStub struct{}

func (graderStub) Grade(_ context.Context, _ application.GradingContext, _ string) (assessment.Evaluation, error) {
	return assessment.Evaluation{
		Score:   2,
		Verdict: "correct",
		CriterionResults: []assessment.CriterionResult{
			{Key: "task_type", Satisfied: true, Explanation: "Названа классификация."},
			{Key: "justification", Satisfied: true, Explanation: "Указаны два класса."},
		},
		Feedback: []string{"Верно.", "Оба критерия выполнены.", "Закрепите тему."},
	}, nil
}

func TestEvaluateRequiresAuthAndReturnsStructuredGrade(t *testing.T) {
	repo := &transcriptionStub{}
	contexts := &contextStub{}
	handler := New(application.New(repo, contexts, nil, graderStub{}))
	requestBody := `{"transcription_id":"tr-1","task_id":"ml_001"}`
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
	if response.Code != http.StatusOK || repo.owner != "student-1" || contexts.taskID != "ml_001" || result.Score != 2 || result.Verdict != "correct" || len(result.CriterionResults) != 2 || len(result.Feedback) != 3 {
		t.Fatalf("response=%d %s, owner=%q, task=%q", response.Code, response.Body.String(), repo.owner, contexts.taskID)
	}
}
