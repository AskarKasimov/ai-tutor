package application

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
)

type BuildFunc func(revision int64, candidates []variant.CandidateOutcome) (variant.Variant, error)

type Cursor struct {
	CreatedAt int64
	ID        string
}

type Repository interface {
	Create(context.Context, string, string, BuildFunc) (variant.Variant, error)
	Get(context.Context, string, string) (variant.Variant, error)
	List(context.Context, string, int, *Cursor) ([]variant.Variant, *Cursor, error)
	Task(context.Context, string, string, string) (variant.VariantTask, error)
	TaskForGrading(context.Context, string, string, string) (variant.TaskProfile, error)
}

type TaskChooser interface {
	Choose(int) (int, error)
}

type IDGenerator interface {
	New(prefix string) (string, error)
}

type Clock interface {
	Unix() int64
}
