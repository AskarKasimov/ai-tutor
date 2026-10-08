package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type stubReader struct {
	result diagnostic.Result
}

func (s *stubReader) Result(_ context.Context, _, _ string) (diagnostic.Result, error) {
	return s.result, nil
}

type stubSynthesizer struct{}

func (s *stubSynthesizer) Synthesize(_ context.Context, _ application.DeterministicReport) (string, error) {
	return "Тестовое резюме.", nil
}

func (s *stubSynthesizer) SynthesizeAnswers(_ context.Context, _ application.SessionFeedbackReport) (string, error) {
	return "Синтез по ответам сессии.", nil
}

func TestGetFeedbackHandler(t *testing.T) {
	reader := &stubReader{
		result: diagnostic.Result{
			SessionID:       "sess-abc",
			DiagnosticScore: 4,
			MaximumScore:    6,
			Answers: []diagnostic.Answer{
				{
					Role:         "main",
					CompetencyID: "comp-1",
					Score:        2,
					Task: diagnostic.TaskSnapshot{
						CompetencyName: "Линейные модели",
						OutcomeName:    "Регрессия",
					},
				},
			},
		},
	}
	svc := application.New(reader, &stubSynthesizer{}, nil, func() time.Time { return time.Unix(100, 0) })
	handler := New(svc)

	req := httptest.NewRequest(http.MethodGet, "/diagnostic-sessions/sess-abc/feedback", nil)
	req.SetPathValue("id", "sess-abc")
	req = httpx.WithPrincipal(req, user.User{ID: "usr-1"})
	rec := httptest.NewRecorder()

	handler.GetFeedback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp OverallFeedbackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.SessionID != "sess-abc" || resp.Summary != "Тестовое резюме." || len(resp.Strengths) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSynthesizeAnswersFeedbackHandler(t *testing.T) {
	svc := application.New(nil, &stubSynthesizer{}, nil, func() time.Time { return time.Unix(100, 0) })
	handler := New(svc)

	body := `{"answers":[{"task_id":"1","question":"Q1","transcript":"ans1","score":2,"max_score":2,"feedback":["good"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/assessments/overall-feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = httpx.WithPrincipal(req, user.User{ID: "usr-1"})
	rec := httptest.NewRecorder()

	handler.SynthesizeAnswersFeedback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp AnswersFeedbackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Score != 2 || resp.MaxScore != 2 || resp.Summary != "Синтез по ответам сессии." {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
