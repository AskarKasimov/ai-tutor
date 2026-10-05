// Package mock provides demonstration grading without a model endpoint.
package mock

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
)

type Grader struct{}

func (Grader) Grade(ctx context.Context, _ application.Task, _ string) (application.Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return application.Evaluation{}, err
	}
	return application.Evaluation{Score: 2, Feedback: []string{
		"Демонстрационная оценка: 2 из 2.",
		"Ответ не проверялся моделью; это фиксированный пример фидбэка.",
		"Демонстрационный результат не отражает уровень знаний.",
	}}, nil
}
