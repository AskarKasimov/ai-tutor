package application

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
)

type BuildFunc func(subjectID, subjectName string, revision int64, candidates []variant.CandidateOutcome) (variant.Variant, error)

type Cursor struct {
	CreatedAt int64
	ID        string
}

type Repository interface {
	Create(context.Context, string, string, string, BuildFunc) (variant.Variant, error)
	Get(context.Context, string, string) (variant.Variant, error)
	List(context.Context, string, string, int, *Cursor) ([]variant.Variant, *Cursor, error)
	Task(context.Context, string, string, string) (variant.VariantTask, error)
	TaskForGrading(context.Context, string, string, string) (variant.VariantTask, error)
}

type TaskChooser interface {
	Choose(int) (int, error)
}

type IDGenerator interface {
	New(prefix string) (string, error)
}
