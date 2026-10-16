package llm

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// Mock is a deterministic examiner for development before the gateway is
// configured. It grades each student answer by length: "не знаю" is don't
// know, under 4 words is weak, under 12 partial, otherwise full. Like the real
// prompt it looks at the whole topic: K1 and K2 are the best over the answers,
// K3 and K4 come from the latest one.
type Mock struct{}

func (Mock) Question(ctx context.Context, r application.QuestionRequest) (string, error) {
	var text string
	switch r.Kind {
	case exam.KindFollowup:
		text = fmt.Sprintf("Хорошо. А в каких условиях подход из темы «%s» перестаёт работать? Приведите контрпример.", r.Node.Title)
	case exam.KindRephrase:
		text = fmt.Sprintf("Попробуем иначе: что вы знаете о теме «%s»? Начните с определения.", r.Node.Title)
	default:
		text = fmt.Sprintf("Расскажите своими словами про тему «%s»: как это работает и зачем нужно?", r.Node.Title)
	}
	httpx.LoggerFromContext(ctx).Info("llm mock question", zap.String("kind", string(r.Kind)), zap.String("node", r.Node.ID), zap.String("text", text))
	return text, nil
}

func (Mock) Evaluate(ctx context.Context, r application.EvaluationRequest) (exam.Evaluation, error) {
	last := r.Dialogue[len(r.Dialogue)-1]
	e := exam.Evaluation{Verdict: exam.VerdictAnswered, Evidence: []int{}}
	if strings.Contains(strings.ToLower(last.Text), "не знаю") {
		e = exam.Evaluation{Verdict: exam.VerdictDontKnow}
	} else {
		for _, t := range r.Dialogue {
			if t.Speaker != exam.SpeakerStudent || strings.Contains(strings.ToLower(t.Text), "не знаю") {
				continue
			}
			c := byLength(t.Text)
			e.Criteria.K1, e.Criteria.K2 = max(e.Criteria.K1, c.K1), max(e.Criteria.K2, c.K2)
			e.Criteria.K3, e.Criteria.K4 = c.K3, c.K4
			e.Evidence = append(e.Evidence, t.Line)
		}
	}
	httpx.LoggerFromContext(ctx).Info("llm mock evaluation", zap.String("node", r.Node.ID), zap.String("answer", last.Text), zap.Any("evaluation", e))
	return e, nil
}

func byLength(text string) exam.Criteria {
	words := len(strings.FieldsFunc(text, func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsDigit(c) }))
	switch {
	case words < 4:
		return exam.Criteria{K1: 0, K2: 1}
	case words < 12:
		return exam.Criteria{K1: 2, K2: 2, K3: 1, K4: 1}
	}
	return exam.Criteria{K1: 2, K2: 2, K3: 2, K4: 2}
}
