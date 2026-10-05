package mock

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
)

type Generator struct{}

func (Generator) Generate(_ context.Context, input application.Context) (application.Draft, error) {
	return application.Draft{
		Question:         "Демонстрационное задание по теме: " + input.Outcome.Name,
		Options:          []string{},
		VoiceInstruction: "Ответьте и кратко объясните ход решения.",
		ReferenceAnswer:  "Вызов модели отключён в mock-режиме.",
	}, nil
}
