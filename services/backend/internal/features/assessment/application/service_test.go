package application

import (
	"context"
	"errors"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type transcriptionStub struct {
	owner, id string
	text      string
}

func (s *transcriptionStub) TextByOwner(_ context.Context, owner, id string) (string, error) {
	s.owner, s.id = owner, id
	return s.text, nil
}

type contextStub struct {
	taskID string
	result GradingContext
	err    error
}

func (s *contextStub) ContextForTask(_ context.Context, taskID string) (GradingContext, error) {
	s.taskID = taskID
	return s.result, s.err
}

type graderStub struct {
	gradingContext GradingContext
	answer         string
	result         Evaluation
}

func (s *graderStub) Grade(_ context.Context, gradingContext GradingContext, answer string) (Evaluation, error) {
	s.gradingContext, s.answer = gradingContext, answer
	return s.result, nil
}

func validContext() GradingContext {
	return GradingContext{
		TaskID: "ml_001", Question: "Что прогнозирует банк?", ReferenceAnswer: "Классификация",
		Criteria: []Criterion{
			{Key: "task_type", Description: "Назван тип задачи"},
			{Key: "justification", Description: "Есть объяснение"},
		},
		MaterialContext: MaterialContext{Knowledge: "K", Skills: "S"},
	}
}

func validEvaluation() Evaluation {
	return Evaluation{
		Score:   2,
		Verdict: "correct",
		CriterionResults: []CriterionResult{
			{Key: "task_type", Satisfied: true, Explanation: "Названа классификация."},
			{Key: "justification", Satisfied: true, Explanation: "Указаны два класса."},
		},
		Feedback: []string{"Верно.", "Оба критерия выполнены.", "Закрепите тему."},
	}
}

func TestEvaluateUsesOwnedTranscriptionAndStructuredCatalogContext(t *testing.T) {
	repo := &transcriptionStub{text: "классификация, потому что два класса"}
	contexts := &contextStub{result: validContext()}
	grader := &graderStub{result: validEvaluation()}
	got, err := New(repo, contexts, grader).Evaluate(context.Background(), "student-1", "tr-1", "ml_001")
	if err != nil || got.Score != 2 || got.Verdict != "correct" || len(got.CriterionResults) != 2 || repo.owner != "student-1" || repo.id != "tr-1" || grader.answer != repo.text {
		t.Fatalf("unexpected evaluation: %#v, %v", got, err)
	}
}

func TestEvaluateRejectsInconsistentStructuredModelOutput(t *testing.T) {
	repo := &transcriptionStub{text: "ответ"}
	contexts := &contextStub{result: validContext()}
	bad := validEvaluation()
	bad.Score = 1
	bad.Verdict = "partial"
	grader := &graderStub{result: bad}
	_, err := New(repo, contexts, grader).Evaluate(context.Background(), "student-1", "tr-1", "ml_001")
	var f *fault.Error
	if !errors.As(err, &f) || f.Kind != fault.Upstream || f.Code != "INVALID_MODEL_RESPONSE" {
		t.Fatalf("expected invalid model response, got %v", err)
	}
}

func TestEvaluateRejectsWrongCriterionKey(t *testing.T) {
	repo := &transcriptionStub{text: "ответ"}
	contexts := &contextStub{result: validContext()}
	bad := validEvaluation()
	bad.CriterionResults[1].Key = "invented"
	_, err := New(repo, contexts, &graderStub{result: bad}).Evaluate(context.Background(), "student-1", "tr-1", "ml_001")
	var f *fault.Error
	if !errors.As(err, &f) || f.Code != "INVALID_MODEL_RESPONSE" {
		t.Fatalf("expected invalid model response, got %v", err)
	}
}

func TestEvaluateStopsWhenCatalogContextIsMissing(t *testing.T) {
	repo := &transcriptionStub{text: "ответ"}
	contexts := &contextStub{err: fault.New(fault.NotFound, "GRADING_CONTEXT_NOT_FOUND", "Контекст не найден.")}
	grader := &graderStub{}
	_, err := New(repo, contexts, grader).Evaluate(context.Background(), "student-1", "tr-1", "missing")
	if err == nil || repo.id != "" || grader.answer != "" {
		t.Fatalf("expected lookup failure before transcription/model access, got %v", err)
	}
}
