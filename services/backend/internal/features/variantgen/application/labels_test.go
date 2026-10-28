package application

import (
	"context"
	"errors"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type failingLabels struct{}

func (failingLabels) Labels(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("gateway is down")
}

func TestGraphLabelsFailureIsRetryableWithoutExposingGatewayDetails(t *testing.T) {
	s := New(nil, nil, nil, nil).WithGraphLabels(failingLabels{})
	_, err := s.GraphLabels(context.Background(), []string{"Тема"})
	var f *fault.Error
	if !errors.As(err, &f) || f.Kind != fault.Unavailable || f.Code != "GRAPH_LABELS_UNAVAILABLE" {
		t.Fatalf("error = %v", err)
	}
	if labels, err := s.GraphLabels(context.Background(), nil); err != nil || len(labels) != 0 {
		t.Fatalf("empty outline called labeler: %v", err)
	}
}
