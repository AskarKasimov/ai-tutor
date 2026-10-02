package taskgenhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type sourceStub struct{}

func (sourceStub) SourceByOutcome(_ context.Context, id string) (application.Source, error) {
	return application.Source{OutcomeID: id, OutcomeName: "Дроби", Tasks: []application.Task{{Question: "Что обозначает дробь?"}}}, nil
}

func (sourceStub) ListOutcomes(_ context.Context) ([]application.Outcome, error) {
	return []application.Outcome{{ID: "o-1", Name: "Дроби", ConstituentName: "Сост", CompetencyName: "ПК-1", TaskCount: 2}}, nil
}

type generatorStub struct{}

func (generatorStub) Generate(_ context.Context, _ application.Source, _ int) ([]application.Task, error) {
	return []application.Task{{Question: "Новое задание?", Criteria: "Верный ответ назван.", VoiceInstruction: "Ответьте голосом."}}, nil
}

func TestGenerateRequiresAuthAndReturnsTasks(t *testing.T) {
	handler := New(application.New(sourceStub{}, generatorStub{}))
	unauthorized := httptest.NewRecorder()
	handler.GenerateTrainingTasks(unauthorized, httptest.NewRequest(http.MethodPost, "/outcomes/o-1/training-tasks", strings.NewReader(`{"count":1}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized request: %d", unauthorized.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/outcomes/o-1/training-tasks", strings.NewReader(`{"count":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.SetPathValue("outcomeId", "o-1")
	request = httpx.WithPrincipal(request, user.User{ID: "student-1", Role: user.Student})
	response := httptest.NewRecorder()
	handler.GenerateTrainingTasks(response, request)
	var result GenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.OutcomeID != "o-1" || len(result.Tasks) != 1 || result.Tasks[0].Question != "Новое задание?" {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestListOutcomesRequiresAuthAndReturnsCatalog(t *testing.T) {
	handler := New(application.New(sourceStub{}, generatorStub{}))
	unauthorized := httptest.NewRecorder()
	handler.ListOutcomes(unauthorized, httptest.NewRequest(http.MethodGet, "/outcomes", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized request: %d", unauthorized.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/outcomes", nil)
	request = httpx.WithPrincipal(request, user.User{ID: "student-1", Role: user.Student})
	response := httptest.NewRecorder()
	handler.ListOutcomes(response, request)
	var result OutcomeList
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(result.Outcomes) != 1 || result.Outcomes[0].TaskCount != 2 {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}
