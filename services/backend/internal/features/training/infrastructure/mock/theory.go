package mock

import (
	"context"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

// Theory builds a deterministic demonstration summary from the source ОС.
type Theory struct{}

func (Theory) Summarize(ctx context.Context, request training.TheoryRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	lines := []string{"Демонстрационная выжимка по теме «" + request.CompetencyName + "»."}
	for _, note := range request.Notes {
		content := strings.Join(strings.Fields(note.Content), " ")
		if runes := []rune(content); len(runes) > 400 {
			content = string(runes[:400]) + "…"
		}
		lines = append(lines, "• "+note.OutcomeName+": "+content)
	}
	return strings.Join(lines, "\n"), nil
}

var _ interface {
	Summarize(context.Context, training.TheoryRequest) (string, error)
} = Theory{}
