package application

import (
	"context"
	"errors"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
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

type variantTaskStub struct {
	ownerID, variantID, variantTaskID string
	result                            variant.VariantTask
	err                               error
}

func (s *variantTaskStub) TaskForGrading(_ context.Context, ownerID, variantID, variantTaskID string) (variant.VariantTask, error) {
	s.ownerID, s.variantID, s.variantTaskID = ownerID, variantID, variantTaskID
	return s.result, s.err
}

type graderStub struct {
	gradingContext GradingContext
	answer         string
	result         assessment.Evaluation
}

func (s *graderStub) Grade(_ context.Context, gradingContext GradingContext, answer string) (assessment.Evaluation, error) {
	s.gradingContext, s.answer = gradingContext, answer
	return s.result, nil
}

func text(value string) *string { return &value }

func validVariantTask(role string) variant.VariantTask {
	return variant.VariantTask{
		ID:   "variant-task-1",
		Role: role,
		Task: variant.TaskProfile{
			ID:               "task-1",
			Question:         "Банк прогнозирует возврат кредита в срок.",
			VoiceInstruction: text("Назовите тип задачи и объясните выбор"),
			ReferenceAnswer:  text("Классификация: целевая переменная имеет два класса."),
			Constituent:      variant.ConstituentProfile{Name: "Типы задач машинного обучения"},
			Outcome:          variant.OutcomeProfile{Name: "Определяет тип задачи"},
		},
	}
}

func validEvaluation() assessment.Evaluation {
	return assessment.Evaluation{
		Score:   2,
		Verdict: "correct",
		CriterionResults: []assessment.CriterionResult{
			{Key: "answer_correctness", Satisfied: true, Explanation: "Ответ соответствует эталону."},
			{Key: "instruction_following", Satisfied: true, Explanation: "Тип назван и выбор объяснён."},
		},
		Feedback: []string{"Верно.", "Оба критерия выполнены.", "Закрепите тему."},
	}
}

func TestEvaluateVariantUsesOwnedHistoricalSnapshot(t *testing.T) {
	transcriptions := &transcriptionStub{text: "классификация, потому что два класса"}
	tasks := &variantTaskStub{result: validVariantTask("main")}
	grader := &graderStub{result: validEvaluation()}

	got, err := New(transcriptions, tasks, grader).EvaluateVariant(
		context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1",
	)
	if err != nil || got.Score != 2 || got.MaxScore != 2 || got.Verdict != "correct" {
		t.Fatalf("unexpected evaluation: %#v, %v", got, err)
	}
	if tasks.ownerID != "student-1" || tasks.variantID != "variant-1" || tasks.variantTaskID != "variant-task-1" {
		t.Fatalf("unexpected snapshot lookup: owner=%q variant=%q task=%q", tasks.ownerID, tasks.variantID, tasks.variantTaskID)
	}
	if transcriptions.owner != "student-1" || transcriptions.id != "tr-1" || grader.answer != transcriptions.text {
		t.Fatalf("unexpected transcription lookup: owner=%q id=%q answer=%q", transcriptions.owner, transcriptions.id, grader.answer)
	}
}

func TestEvaluateVariantBasicUsesOnePointScale(t *testing.T) {
	result := validEvaluation()
	result.Score = 1
	result.Verdict = "correct"

	got, err := New(
		&transcriptionStub{text: "классификация, потому что два класса"},
		&variantTaskStub{result: validVariantTask("basic")},
		&graderStub{result: result},
	).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")

	if err != nil || got.Score != 1 || got.MaxScore != 1 || got.Verdict != "correct" {
		t.Fatalf("expected basic 1/1 correct, got %#v, %v", got, err)
	}
}

func TestEvaluateVariantRejectsTwoPointsForBasic(t *testing.T) {
	_, err := New(
		&transcriptionStub{text: "классификация, потому что два класса"},
		&variantTaskStub{result: validVariantTask("basic")},
		&graderStub{result: validEvaluation()},
	).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")

	var f *fault.Error
	if !errors.As(err, &f) || f.Code != "INVALID_MODEL_RESPONSE" {
		t.Fatalf("expected INVALID_MODEL_RESPONSE for basic score 2, got %v", err)
	}
}

func TestEvaluateVariantWorksWithoutCriteriaOrEducationalContent(t *testing.T) {
	item := validVariantTask("main")
	item.Task.Criteria = nil
	item.Task.Outcome.EducationalContent = nil

	grader := &graderStub{result: validEvaluation()}
	got, err := New(
		&transcriptionStub{text: "классификация, потому что два класса"},
		&variantTaskStub{result: item},
		grader,
	).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")

	if err != nil || got.Score != 2 {
		t.Fatalf("expected grading from sparse historical snapshot, got %#v, %v", got, err)
	}
	if grader.gradingContext.MaterialContext.Knowledge != "" {
		t.Fatalf("expected empty optional educational content, got %q", grader.gradingContext.MaterialContext.Knowledge)
	}
	if len(grader.gradingContext.Criteria) != 2 {
		t.Fatalf("expected generated grading criteria, got %#v", grader.gradingContext.Criteria)
	}
}

func TestEvaluateVariantMandatoryCriterionFailureForcesZero(t *testing.T) {
	result := validEvaluation()
	result.Score = 0
	result.Verdict = "incorrect"
	result.CriterionResults[1] = CriterionResult{
		Key:         "instruction_following",
		Satisfied:   false,
		Explanation: "Тип задачи словами не назван.",
	}

	got, err := New(
		&transcriptionStub{text: "Потому что целевая переменная принимает два класса."},
		&variantTaskStub{result: validVariantTask("main")},
		&graderStub{result: result},
	).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")

	if err != nil || got.Score != 0 || got.Verdict != "incorrect" {
		t.Fatalf("expected mandatory failure to produce 0/incorrect, got %#v, %v", got, err)
	}
}

func TestEvaluateVariantRejectsPartialWhenMandatoryCriterionFailed(t *testing.T) {
	result := validEvaluation()
	result.Score = 1
	result.Verdict = "partial"
	result.CriterionResults[1].Satisfied = false
	result.CriterionResults[1].Explanation = "Тип задачи словами не назван."

	_, err := New(
		&transcriptionStub{text: "Потому что целевая переменная принимает два класса."},
		&variantTaskStub{result: validVariantTask("main")},
		&graderStub{result: result},
	).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")

	var f *fault.Error
	if !errors.As(err, &f) || f.Code != "INVALID_MODEL_RESPONSE" {
		t.Fatalf("expected INVALID_MODEL_RESPONSE, got %v", err)
	}
}

func TestEvaluateVariantStopsWhenSnapshotLookupFails(t *testing.T) {
	transcriptions := &transcriptionStub{text: "ответ"}
	tasks := &variantTaskStub{err: fault.New(fault.NotFound, "VARIANT_TASK_NOT_FOUND", "Задание варианта не найдено.")}
	grader := &graderStub{}

	_, err := New(transcriptions, tasks, grader).EvaluateVariant(
		context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1",
	)
	if err == nil || transcriptions.id != "" || grader.answer != "" {
		t.Fatalf("expected snapshot lookup failure before transcription/model access, got %v", err)
	}
}
