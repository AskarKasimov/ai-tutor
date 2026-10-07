// Package mock provides demonstration grading without a model endpoint.
package mock

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
)

type Grader struct{}

func (Grader) Grade(ctx context.Context, gradingContext application.GradingContext, _ string) (application.Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return application.Evaluation{}, err
	}
	results := make([]application.CriterionResult, len(gradingContext.Criteria))
	for i, criterion := range gradingContext.Criteria {
		results[i] = application.CriterionResult{Key: criterion.Key, Satisfied: true, Explanation: "Демонстрационный результат критерия."}
	}
	score := gradingContext.MaxScore
	if score != 1 {
		score = 2
	}
	return application.Evaluation{
		Score:            score,
		Verdict:          "correct",
		CriterionResults: results,
		Feedback: []string{
			"Демонстрационная оценка.",
			"Ответ не проверялся моделью; это фиксированный пример фидбэка.",
			"Демонстрационный результат не отражает уровень знаний.",
		},
	}, nil
}
