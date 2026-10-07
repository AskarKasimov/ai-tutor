package mock

import (
	"context"
	"fmt"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
)

type Synthesizer struct{}

func New() *Synthesizer {
	return &Synthesizer{}
}

func (s *Synthesizer) Synthesize(_ context.Context, report application.DeterministicReport) (string, error) {
	var sb strings.Builder

	switch {
	case report.ScorePercentage >= 85:
		sb.WriteString(fmt.Sprintf("Отличный результат! Вы набрали %d из %d баллов (%d%%). Продемонстрировано уверенное понимание большинства тем среза.",
			report.DiagnosticScore, report.MaximumScore, report.ScorePercentage))
	case report.ScorePercentage >= 60:
		sb.WriteString(fmt.Sprintf("Хороший результат: %d из %d баллов (%d%%). Базовые принципы понятны, но в некоторых темах требуется более глубокая проработка деталей и аргументации.",
			report.DiagnosticScore, report.MaximumScore, report.ScorePercentage))
	case report.ScorePercentage >= 40:
		sb.WriteString(fmt.Sprintf("Удовлетворительный результат: %d из %d баллов (%d%%). Выявлены ключевые пробелы в фундаментальных понятиях, требующие систематического повторения.",
			report.DiagnosticScore, report.MaximumScore, report.ScorePercentage))
	default:
		sb.WriteString(fmt.Sprintf("Диагностический срез показал существенные сложности: %d из %d баллов (%d%%). Рекомендуется начать повторение с базовых определений и определителей задач.",
			report.DiagnosticScore, report.MaximumScore, report.ScorePercentage))
	}

	if len(report.Strengths) > 0 {
		sb.WriteString(" Сильные стороны: вы успешно справились с заданиями высокого уровня в освоенных темах.")
	}

	if len(report.ConfirmedGaps) > 0 {
		gapNames := make([]string, 0, len(report.ConfirmedGaps))
		for _, g := range report.ConfirmedGaps {
			gapNames = append(gapNames, fmt.Sprintf("«%s»", g.OutcomeName))
		}
		sb.WriteString(fmt.Sprintf(" Основное внимание при тренировке следует уделить результатам: %s.", strings.Join(gapNames, ", ")))
	} else if len(report.PartialCompetencies) > 0 {
		sb.WriteString(" Базовые определения усвоены, сфокусируйтесь на практике применения и объяснении выбора.")
	}

	return sb.String(), nil
}

func (s *Synthesizer) SynthesizeAnswers(_ context.Context, report application.SessionFeedbackReport) (string, error) {
	var sb strings.Builder

	switch {
	case report.ScorePercentage >= 85:
		sb.WriteString(fmt.Sprintf("Отличный результат! Вы набрали %d из %d баллов (%d%%). Продемонстрировано уверенное понимание большинства проверенных тем курса.",
			report.Score, report.MaxScore, report.ScorePercentage))
	case report.ScorePercentage >= 60:
		sb.WriteString(fmt.Sprintf("Хороший результат: %d из %d баллов (%d%%). Базовые принципы понятны, но в некоторых темах требуется глубже проработать аргументацию.",
			report.Score, report.MaxScore, report.ScorePercentage))
	case report.ScorePercentage >= 40:
		sb.WriteString(fmt.Sprintf("Удовлетворительный результат: %d из %d баллов (%d%%). Выявлены ключевые пробелы, требующие повторения теории и решения задач.",
			report.Score, report.MaxScore, report.ScorePercentage))
	default:
		sb.WriteString(fmt.Sprintf("Результат сессии: %d из %d баллов (%d%%). Рекомендуется начать повторение с базовых определений и типов задач ML.",
			report.Score, report.MaxScore, report.ScorePercentage))
	}

	if len(report.Strengths) > 0 {
		sb.WriteString(" Сильные стороны: вы успешно справились с ключевыми заданиями среза.")
	}
	if len(report.Gaps) > 0 {
		sb.WriteString(" Обратите внимание на темы с нулевым баллом — они требуют первоочередного внимания.")
	} else if len(report.Partials) > 0 {
		sb.WriteString(" Для тем с частичным баллом рекомендуется дополнительно попрактиковаться.")
	}

	return sb.String(), nil
}
