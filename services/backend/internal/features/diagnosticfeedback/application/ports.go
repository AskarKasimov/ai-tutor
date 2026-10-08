package application

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

type ResultReader interface {
	Result(ctx context.Context, ownerID, sessionID string) (diagnostic.Result, error)
}

type FeedbackSynthesizer interface {
	Synthesize(ctx context.Context, report DeterministicReport) (string, error)
	SynthesizeAnswers(ctx context.Context, report SessionFeedbackReport) (string, error)
}

type FeedbackCache interface {
	Get(ctx context.Context, sessionID string) (*diagnostic.OverallFeedback, bool)
	Set(ctx context.Context, sessionID string, feedback diagnostic.OverallFeedback)
}

type TaskMetadataFinder interface {
	FindTaskMetadata(ctx context.Context, taskID string) (competencyName, outcomeName, educationalContent string, err error)
}
