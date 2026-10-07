package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Evaluation struct {
	Score            int               `json:"score"`
	MaxScore         int               `json:"max_score"`
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
	variantTasks   variant.TaskReader
	grader         Grader
}

func New(transcriptions Transcriptions, contexts ContextProvider, variantTasks variant.TaskReader, grader Grader) *Service {
	return &Service{
		transcriptions: transcriptions,
		contexts:       contexts,
		variantTasks:   variantTasks,
		grader:         grader,
	}
}

func validText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}

func gradingMaxScore(gradingContext GradingContext) int {
	if gradingContext.MaxScore == 1 {
		return 1
	}
	return 2
}

func expectedVerdict(score, maxScore int) string {
	if maxScore == 1 {
		switch score {
		case 1:
			return "correct"
		case 0:
			return "incorrect"
		default:
			return ""
		}
	}
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
	maxScore := gradingMaxScore(gradingContext)
	if result.Score < 0 || result.Score > maxScore || result.Verdict != expectedVerdict(result.Score, maxScore) {
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

	expectedScore := 0
	if maxScore == 1 {
		if satisfied == len(gradingContext.Criteria) {
			expectedScore = 1
		}
	} else if satisfied == len(gradingContext.Criteria) {
		expectedScore = 2
	} else if satisfied > 0 {
		expectedScore = 1
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

func (s *Service) evaluateWithContext(ctx context.Context, ownerID, transcriptionID string, gradingContext GradingContext) (Evaluation, error) {
	if !validText(transcriptionID, 128) {
		return Evaluation{}, fault.Validation("transcription_id", "Укажите сохранённую расшифровку.")
	}
	if gradingContext.MaxScore == 0 {
		gradingContext.MaxScore = 2
	}
	if gradingContext.Role == "" {
		gradingContext.Role = "main"
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
	result.MaxScore = gradingMaxScore(gradingContext)
	return result, nil
}

// Evaluate keeps the fixed prototype task_id flow working until the frontend switches to saved variants.
func (s *Service) Evaluate(ctx context.Context, ownerID, transcriptionID, taskID string) (Evaluation, error) {
	if !validText(taskID, 128) {
		return Evaluation{}, fault.Validation("task_id", "Укажите идентификатор задания.")
	}
	if s.contexts == nil {
		return Evaluation{}, fault.New(fault.Unavailable, "GRADING_CONTEXT_UNAVAILABLE", "Контекст оценивания временно недоступен.")
	}
	gradingContext, err := s.contexts.ContextForTask(ctx, taskID)
	if err != nil {
		return Evaluation{}, err
	}
	return s.evaluateWithContext(ctx, ownerID, transcriptionID, gradingContext)
}

func (s *Service) EvaluateVariant(ctx context.Context, ownerID, transcriptionID, variantID, variantTaskID string) (Evaluation, error) {
	if !validText(variantID, 128) {
		return Evaluation{}, fault.Validation("variant_id", "Укажите идентификатор варианта.")
	}
	if !validText(variantTaskID, 128) {
		return Evaluation{}, fault.Validation("variant_task_id", "Укажите идентификатор позиции задания варианта.")
	}
	if s.variantTasks == nil {
		return Evaluation{}, fault.New(fault.Unavailable, "VARIANT_GRADING_UNAVAILABLE", "Оценивание заданий варианта временно недоступно.")
	}

	item, err := s.variantTasks.TaskForGrading(ctx, ownerID, variantID, variantTaskID)
	if err != nil {
		return Evaluation{}, err
	}
	gradingContext, err := GradingContextFromVariantTask(item)
	if err != nil {
		return Evaluation{}, err
	}
	return s.evaluateWithContext(ctx, ownerID, transcriptionID, gradingContext)
}
