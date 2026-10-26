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

// FeedbackStore keeps the overall feedback of a diagnostic once it is
// generated, so the student always reads the same text.
type FeedbackStore interface {
	Get(ctx context.Context, ownerID, sessionID string) (diagnostic.OverallFeedback, bool, error)
	Save(ctx context.Context, ownerID string, feedback diagnostic.OverallFeedback) error
}

type TaskMetadataFinder interface {
	FindTaskMetadata(ctx context.Context, taskID string) (competencyName, outcomeName, educationalContent string, err error)
}
