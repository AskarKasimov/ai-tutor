package snapshot

import (
	"context"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

type Provider struct{}

func (Provider) Generate(_ context.Context, target training.Target, _ []training.Attempt) (training.Task, string, error) {
	t := target.Source
	t.Options = append([]string{}, t.Options...)
	return t, "snapshot-v1", nil
}
