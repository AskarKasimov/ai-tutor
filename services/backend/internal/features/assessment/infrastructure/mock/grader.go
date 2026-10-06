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
	return application.Evaluation{
		Score:            2,
		Verdict:          "correct",
		CriterionResults: results,
		Feedback: []string{
			"Демонстрационная оценка: 2 из 2.",
			"Ответ не проверялся моделью; это фиксированный пример фидбэка.",
			"Демонстрационный результат не отражает уровень знаний.",
		},
	}, nil
}
