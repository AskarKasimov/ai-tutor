package application

import (
	"context"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

// GraphLabeler generates display names without changing the source map.
type GraphLabeler interface {
	Labels(context.Context, []string) (map[string]string, error)
	RequestBudget(int) time.Duration
}

func (s *Service) GraphLabelsBudget(titleCount int) time.Duration {
	if s.labels == nil || titleCount == 0 {
		return 0
	}
	return s.labels.RequestBudget(titleCount)
}

func (s *Service) WithGraphLabels(labels GraphLabeler) *Service {
	s.labels = labels
	return s
}

func (s *Service) GraphLabels(ctx context.Context, names []string) (map[string]string, error) {
	if s.labels == nil || len(names) == 0 {
		return nil, nil
	}
	labels, err := s.labels.Labels(ctx, names)
	if err != nil {
		return nil, fault.New(fault.Unavailable, "GRAPH_LABELS_UNAVAILABLE", "Не удалось подготовить короткие названия тем. Попробуйте ещё раз.")
	}
	return labels, nil
}
