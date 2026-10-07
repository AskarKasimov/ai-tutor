package application

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
)

type VariantReader interface {
	Get(context.Context, string, string) (variant.Variant, error)
}

type Voice interface {
	Transcribe(context.Context, string, []byte, string) (transcription.Transcription, error)
	Synthesize(context.Context, string) ([]byte, error)
}

type Grader interface {
	EvaluateVariant(context.Context, string, string, string, string) (assessment.Evaluation, error)
}

type Store interface {
	FindStart(context.Context, string, string, string) (diagnostic.Session, bool, error)
	Create(context.Context, string, string, string, diagnostic.Session) (diagnostic.Session, bool, error)
	Get(context.Context, string, string) (diagnostic.Session, error)
	Reserve(context.Context, string, string, string, string, string, string) (*diagnostic.AcceptedRequest, *diagnostic.Reservation, error)
	Accept(context.Context, string, string, string, string, string, diagnostic.Answer, diagnostic.Transition) (diagnostic.Progress, error)
	Fail(context.Context, string, string, string, string, string) error
}

type IDGenerator interface {
	New(string) (string, error)
}
