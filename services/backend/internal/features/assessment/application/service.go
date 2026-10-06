package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Evaluation struct {
	Score            int               `json:"score"`
	Verdict          string            `json:"verdict"`
	CriterionResults []CriterionResult `json:"criterion_results"`
	Feedback         []string          `json:"feedback"`
}

type Transcriptions interface {
	TextByOwner(context.Context, string, string) (string, error)
}

type Grader interface {
	Grade(context.Context, GradingContext, string) (Evaluation, error)
}

type Service struct {
	transcriptions Transcriptions
	contexts       ContextProvider
	grader         Grader
}

func New(transcriptions Transcriptions, contexts ContextProvider, grader Grader) *Service {
	return &Service{transcriptions: transcriptions, contexts: contexts, grader: grader}
}

func validText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}

func expectedVerdict(score int) string {
	switch score {
	case 2:
		return "correct"
	case 1:
		return "partial"
	case 0:
		return "incorrect"
	default:
		return ""
	}
}

func validateEvaluation(result Evaluation, gradingContext GradingContext) error {
	if result.Score < 0 || result.Score > 2 || result.Verdict != expectedVerdict(result.Score) {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректную оценку.")
	}
	if len(result.CriterionResults) != len(gradingContext.Criteria) || len(gradingContext.Criteria) == 0 {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула неполные результаты по критериям.")
	}

	satisfied := 0
	for i, criterion := range gradingContext.Criteria {
		item := result.CriterionResults[i]
		if item.Key != criterion.Key || !validText(item.Explanation, 400) || strings.ContainsAny(item.Explanation, "\r\n") {
			return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректные результаты по критериям.")
		}
		if item.Satisfied {
			satisfied++
		}
		result.CriterionResults[i].Explanation = strings.TrimSpace(item.Explanation)
	}

	expectedScore := 1
	if satisfied == 0 {
		expectedScore = 0
	} else if satisfied == len(gradingContext.Criteria) {
		expectedScore = 2
	}
	if result.Score != expectedScore {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Оценка модели не согласуется с результатами по критериям.")
	}

	if len(result.Feedback) != 3 {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный фидбэк.")
	}
	for i, line := range result.Feedback {
		line = strings.TrimSpace(line)
		if !validText(line, 240) || strings.ContainsAny(line, "\r\n") {
			return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный фидбэк.")
		}
		result.Feedback[i] = line
	}
	return nil
}

func (s *Service) Evaluate(ctx context.Context, ownerID, transcriptionID, taskID string) (Evaluation, error) {
	if !validText(transcriptionID, 128) {
		return Evaluation{}, fault.Validation("transcription_id", "Укажите сохранённую расшифровку.")
	}
	if !validText(taskID, 128) {
		return Evaluation{}, fault.Validation("task_id", "Укажите идентификатор задания.")
	}

	gradingContext, err := s.contexts.ContextForTask(ctx, taskID)
	if err != nil {
		return Evaluation{}, err
	}
	answer, err := s.transcriptions.TextByOwner(ctx, ownerID, transcriptionID)
	if err != nil {
		return Evaluation{}, err
	}
	if !validText(answer, 20000) {
		return Evaluation{}, fault.New(fault.Invalid, "INVALID_TRANSCRIPTION", "Расшифровка слишком длинная или повреждена.")
	}
	result, err := s.grader.Grade(ctx, gradingContext, answer)
	if err != nil {
		return Evaluation{}, err
	}
	if err := validateEvaluation(result, gradingContext); err != nil {
		return Evaluation{}, err
	}
	return result, nil
}
