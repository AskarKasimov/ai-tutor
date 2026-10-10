package application

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Evaluation = assessment.Evaluation

type CriterionResult = assessment.CriterionResult

type Transcriptions interface {
	TextByOwner(context.Context, string, string) (string, error)
}

type Grader interface {
	Grade(context.Context, GradingContext, string) (Evaluation, error)
}

type Service struct {
	transcriptions Transcriptions
	variantTasks   variant.TaskReader
	grader         Grader
}

func New(transcriptions Transcriptions, variantTasks variant.TaskReader, grader Grader) *Service {
	return &Service{
		transcriptions: transcriptions,
		variantTasks:   variantTasks,
		grader:         grader,
	}
}

func validText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}

func gradingMaxScore(gradingContext GradingContext) (int, error) {
	switch {
	case (gradingContext.Role == "main" || gradingContext.Role == "training") && gradingContext.MaxScore == 2:
		return 2, nil
	case gradingContext.Role == "basic" && gradingContext.MaxScore == 1:
		return 1, nil
	default:
		return 0, fault.New(
			fault.Invalid,
			"INVALID_GRADING_CONTEXT",
			"Контекст оценивания содержит некорректную роль или шкалу.",
		)
	}
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

func shortenFeedback(feedback []string) {
	const maxRunes = 228
	lengths := make([]int, len(feedback))
	total := len(feedback) - 1 // Spaces joining the three sentences.
	for i, line := range feedback {
		lengths[i] = utf8.RuneCountInString(line)
		total += lengths[i]
	}
	if total <= maxRunes {
		return
	}
	for total > maxRunes {
		longest := 0
		for i := 1; i < len(lengths); i++ {
			if lengths[i] > lengths[longest] {
				longest = i
			}
		}
		lengths[longest]--
		total--
	}
	for i, line := range feedback {
		if utf8.RuneCountInString(line) <= lengths[i] {
			continue
		}
		runes := []rune(line)
		prefix := string(runes[:lengths[i]-1]) // Reserve one rune for the ellipsis.
		if cut := strings.LastIndexFunc(prefix, unicode.IsSpace); cut >= 0 &&
			utf8.RuneCountInString(prefix[:cut]) >= lengths[i]/2 {
			prefix = prefix[:cut]
		}
		cleaned := strings.TrimRight(prefix, " \t,;:.!?—–-")
		if cleaned == "" {
			cleaned = strings.TrimRight(prefix, " \t")
		}
		feedback[i] = cleaned + "…"
	}
}

func validateEvaluation(result Evaluation, gradingContext GradingContext) error {
	maxScore, err := gradingMaxScore(gradingContext)
	if err != nil {
		return err
	}
	if result.Score < 0 || result.Score > maxScore || result.Verdict != expectedVerdict(result.Score, maxScore) {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректную оценку.")
	}
	if len(result.CriterionResults) != len(gradingContext.Criteria) || len(gradingContext.Criteria) == 0 {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула неполные результаты по критериям.")
	}

	satisfied := 0
	mandatoryFailed := false
	for i, criterion := range gradingContext.Criteria {
		item := result.CriterionResults[i]
		if item.Key != criterion.Key || !validText(item.Explanation, 400) || strings.ContainsAny(item.Explanation, "\r\n") {
			return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректные результаты по критериям.")
		}
		if item.Satisfied {
			satisfied++
		}
		if criterion.Mandatory && !item.Satisfied {
			mandatoryFailed = true
		}
		result.CriterionResults[i].Explanation = strings.TrimSpace(item.Explanation)
	}

	expectedScore := 0
	if !mandatoryFailed {
		if maxScore == 1 {
			if satisfied == len(gradingContext.Criteria) {
				expectedScore = 1
			}
		} else if satisfied == len(gradingContext.Criteria) {
			expectedScore = 2
		} else if satisfied > 0 {
			expectedScore = 1
		}
	}
	if result.Score != expectedScore {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Оценка модели не согласуется с результатами по критериям.")
	}

	if len(result.Feedback) != 3 {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный фидбэк.")
	}
	for i, line := range result.Feedback {
		line = strings.TrimSpace(line)
		if line == "" || !utf8.ValidString(line) || strings.ContainsAny(line, "\x00\r\n") {
			return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный фидбэк.")
		}
		result.Feedback[i] = line
	}
	shortenFeedback(result.Feedback)
	return nil
}

func (s *Service) evaluateWithContext(ctx context.Context, ownerID, transcriptionID string, gradingContext GradingContext) (Evaluation, error) {
	if !validText(transcriptionID, 128) {
		return Evaluation{}, fault.Validation("transcription_id", "Укажите сохранённую расшифровку.")
	}
	maxScore, err := gradingMaxScore(gradingContext)
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
	result.MaxScore = maxScore
	return result, nil
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

// EvaluateTrusted evaluates a server-owned snapshot; HTTP clients cannot supply it.
func (s *Service) EvaluateTrusted(ctx context.Context, ownerID, transcriptionID string, item variant.VariantTask) (Evaluation, error) {
	gradingContext, err := GradingContextFromVariantTask(item)
	if err != nil {
		return Evaluation{}, err
	}
	return s.evaluateWithContext(ctx, ownerID, transcriptionID, gradingContext)
}
