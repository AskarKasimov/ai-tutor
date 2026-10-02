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

type graderStub struct {
	task   Task
	answer string
	result Evaluation
}

func (s *graderStub) Grade(_ context.Context, task Task, answer string) (Evaluation, error) {
	s.task, s.answer = task, answer
	return s.result, nil
}

func TestEvaluateUsesOwnedTranscriptionAndTask(t *testing.T) {
	repo := &transcriptionStub{text: "классификация, потому что два класса"}
	grader := &graderStub{result: Evaluation{Score: 2, Feedback: []string{"Верно.", "Два класса.", "Закрепите тему."}}}
	task := Task{Question: "Что прогнозирует банк?", Options: []string{"Классификация", "Регрессия"}, VoiceInstruction: "Назовите тип и объясните.", CorrectAnswer: "Классификация"}
	got, err := New(repo, grader).Evaluate(context.Background(), "student-1", "tr-1", task)
	if err != nil || got.Score != 2 || repo.owner != "student-1" || repo.id != "tr-1" || grader.answer != repo.text || grader.task.CorrectAnswer != task.CorrectAnswer {
		t.Fatalf("unexpected evaluation: %#v, %v", got, err)
	}
}

func TestEvaluateRejectsInvalidTaskAndModelOutput(t *testing.T) {
	repo := &transcriptionStub{text: "ответ"}
	grader := &graderStub{result: Evaluation{Score: 3, Feedback: []string{"line 1", "line 2", "line 3"}}}
	service := New(repo, grader)
	_, err := service.Evaluate(context.Background(), "student-1", "tr-1", Task{Question: "", VoiceInstruction: "Инструкция"})
	var f *fault.Error
	if !errors.As(err, &f) || f.Kind != fault.Invalid || repo.id != "" {
		t.Fatalf("expected input validation before database access, got %v", err)
	}
	_, err = service.Evaluate(context.Background(), "student-1", "tr-1", Task{Question: "Вопрос", VoiceInstruction: "Инструкция"})
	if !errors.As(err, &f) || f.Kind != fault.Upstream {
		t.Fatalf("expected invalid model score to fail, got %v", err)
	}
}
