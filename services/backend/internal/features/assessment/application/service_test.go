package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

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
	result         Evaluation
}

func (s *graderStub) Grade(_ context.Context, gradingContext GradingContext, answer string) (Evaluation, error) {
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

func validEvaluation() Evaluation {
	return Evaluation{
		Score:   2,
		Verdict: "correct",
		CriterionResults: []CriterionResult{
			{Key: "answer_correctness", Satisfied: true, Explanation: "Ответ соответствует эталону."},
			{Key: "instruction_following", Satisfied: true, Explanation: "Тип назван и выбор объяснён."},
		},
		Feedback: []string{"Верно.", "Оба критерия выполнены.", "Закрепите тему."},
	}
}
func TestTrustedTrainingSnapshotUsesTwoPointScale(t *testing.T) {
	for _, score := range []int{0, 1, 2} {
		t.Run(string(rune('0'+score)), func(t *testing.T) {
			result := validEvaluation()
			if score == 0 {
				result.Score = 0
				result.Verdict = "incorrect"
				result.CriterionResults[0].Satisfied = false
				result.CriterionResults[1].Satisfied = false
			}
			if score == 1 {
				result.Score = 1
				result.Verdict = "partial"
				result.CriterionResults[0].Satisfied = false
			}
			grader := &graderStub{result: result}
			service := New(&transcriptionStub{text: "Ответ"}, nil, grader)
			got, err := service.EvaluateTrusted(context.Background(), "owner", "transcription", validVariantTask("training"))
			if err != nil || got.Score != score || got.MaxScore != 2 || grader.gradingContext.Role != "training" {
				t.Fatalf("score %d: %+v %v", score, got, err)
			}
		})
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

func TestEvaluateVariantRejectsInconsistentModelOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Evaluation)
	}{
		{"out of range score", func(e *Evaluation) { e.Score = 3 }},
		{"inconsistent verdict", func(e *Evaluation) { e.Verdict = "partial" }},
		{"wrong criterion key", func(e *Evaluation) { e.CriterionResults[0].Key = "other" }},
		{"missing criterion", func(e *Evaluation) { e.CriterionResults = e.CriterionResults[:1] }},
		{"missing feedback", func(e *Evaluation) { e.Feedback = e.Feedback[:2] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validEvaluation()
			tc.mutate(&result)
			_, err := New(&transcriptionStub{text: "ответ"}, &variantTaskStub{result: validVariantTask("main")}, &graderStub{result: result}).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")
			var f *fault.Error
			if !errors.As(err, &f) || f.Code != "INVALID_MODEL_RESPONSE" {
				t.Fatalf("expected INVALID_MODEL_RESPONSE, got %v", err)
			}
		})
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

func TestEvaluateVariantFeedbackTotalLength(t *testing.T) {
	tests := []struct {
		name      string
		feedback  []string
		shortened bool
	}{
		{name: "brief feedback", feedback: []string{"Названы два класса.", "Но тип задачи не указан.", "Добавьте слово «классификация»."}},
		{name: "exactly 180 runes", feedback: []string{strings.Repeat("а", 174), "б.", "в."}},
		{name: "181 runes stays untouched", feedback: []string{strings.Repeat("а", 175), "б.", "в."}},
		{name: "exactly 228 runes stays untouched", feedback: []string{strings.Repeat("а", 222), "б.", "в."}},
		{name: "229 runes gets shortened", feedback: []string{strings.Repeat("а", 223), "б.", "в."}, shortened: true},
		{name: "emoji unicode count", feedback: []string{strings.Repeat("🙂", 223), "б.", "в."}, shortened: true},
		{name: "500 unicode runes", feedback: []string{strings.Repeat("я🙂", 247), "б.", "в."}, shortened: true},
		{name: "single oversized sentence", feedback: []string{strings.Repeat("Ответ охватывает несколько деталей задания. ", 10), "Есть неточность.", "Уточните правило."}, shortened: true},
		{name: "three oversized sentences", feedback: []string{strings.Repeat("Назван правильный механизм. ", 12), strings.Repeat("Причина оценки подробно разобрана. ", 12), strings.Repeat("Проверьте отдельные примеры. ", 12)}, shortened: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validEvaluation()
			result.Feedback = append([]string(nil), tc.feedback...)
			got, err := New(
				&transcriptionStub{text: "классификация, потому что два класса"},
				&variantTaskStub{result: validVariantTask("main")},
				&graderStub{result: result},
			).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")
			if err != nil {
				t.Fatalf("valid grading must not fail solely due to feedback length: %v", err)
			}
			if got.Score != result.Score || got.MaxScore != 2 || got.Verdict != result.Verdict || !reflect.DeepEqual(got.CriterionResults, result.CriterionResults) {
				t.Fatalf("feedback shortening changed grading: %+v", got)
			}
			joined := strings.Join(got.Feedback, " ")
			if len(got.Feedback) != 3 || utf8.RuneCountInString(joined) > 228 || !utf8.ValidString(joined) {
				t.Fatalf("expected valid three-part feedback within 228 runes, got %q", joined)
			}
			for _, part := range got.Feedback {
				if strings.TrimSpace(part) == "" {
					t.Fatalf("feedback contains an empty part: %#v", got.Feedback)
				}
			}
			original := strings.Join(tc.feedback, " ")
			if tc.shortened && (joined == original || !strings.Contains(joined, "…")) {
				t.Fatalf("expected shortened feedback, got %q", joined)
			}
			if !tc.shortened && joined != original {
				t.Fatalf("feedback under the hard cap was modified: %q", joined)
			}
		})
	}
}

func TestShortenFeedbackUsesWordBoundaries(t *testing.T) {
	feedback := []string{strings.Repeat("Правильно названа классификация. ", 12), "Причина верна.", "Повторите определение."}
	original := feedback[0]
	shortenFeedback(feedback)
	prefix := strings.TrimSuffix(feedback[0], "…")
	if !strings.HasPrefix(original, prefix+" ") && !strings.HasPrefix(original, prefix+". ") {
		t.Fatalf("feedback was cut inside a word: %q", feedback[0])
	}
}

func TestEvaluateTrustedTrainingOversizedFeedback(t *testing.T) {
	result := validEvaluation()
	result.Feedback = []string{
		strings.Repeat("Правильный пример приведён. ", 15),
		strings.Repeat("Уточните формулировку. ", 15),
		strings.Repeat("Сравните с эталоном. ", 15),
	}
	got, err := New(
		&transcriptionStub{text: "Пример ответа"}, nil, &graderStub{result: result},
	).EvaluateTrusted(context.Background(), "student-1", "tr-1", validVariantTask("training"))
	if err != nil || got.Score != 2 || got.MaxScore != 2 {
		t.Fatalf("training grade changed during feedback shortening: %+v %v", got, err)
	}
	if len(got.Feedback) != 3 || utf8.RuneCountInString(strings.Join(got.Feedback, " ")) > 228 {
		t.Fatalf("training feedback exceeds 228 runes: %#v", got.Feedback)
	}
}

func TestEvaluateVariantRejectsMalformedFeedback(t *testing.T) {
	for _, feedback := range [][]string{
		{"", "Причина.", "Совет."},
		{"Наблюдение.\nСледующая строка.", "Причина.", "Совет."},
		{"Наблюдение.\rПричина.", "Причина.", "Совет."},
		{"Наблюдение.\x00Пробел", "Причина.", "Совет."},
		{string([]byte{0xff}), "Причина.", "Совет."},
	} {
		result := validEvaluation()
		result.Feedback = feedback
		_, err := New(
			&transcriptionStub{text: "классификация, потому что два класса"},
			&variantTaskStub{result: validVariantTask("main")},
			&graderStub{result: result},
		).EvaluateVariant(context.Background(), "student-1", "tr-1", "variant-1", "variant-task-1")
		var f *fault.Error
		if !errors.As(err, &f) || f.Code != "INVALID_MODEL_RESPONSE" {
			t.Fatalf("expected malformed feedback to be rejected, got %v", err)
		}
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
