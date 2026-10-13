package application

import (
	"context"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

type scoreDiagnostics struct{ session diagnostic.Session }

func (d scoreDiagnostics) Get(context.Context, string, string) (diagnostic.Session, error) {
	return d.session, nil
}

type scorePracticeRepo struct{ Repository }

func (scorePracticeRepo) Practice(context.Context, string) (int64, []training.Task, error) {
	return 1, []training.Task{{OutcomeID: "main"}}, nil
}

func TestPreviewUsesDiagnosticScoreAndStartsFreePracticeAfterPerfectMain(t *testing.T) {
	for _, tc := range []struct {
		name              string
		main, basic, want int
		mode              string
	}{
		{"perfect", 3, -1, 3, "free_practice"},
		{"partial with correct basic", 1, 1, 2, "focused"},
		{"legacy perfect", 2, -1, 2, "free_practice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answers := []diagnostic.Answer{{Role: "main", CompetencyID: "c", OutcomeID: "main", Score: tc.main, Task: diagnostic.TaskSnapshot{ID: "main", OutcomeID: "main"}}}
			if tc.basic >= 0 {
				answers = append(answers, diagnostic.Answer{Role: "basic", CompetencyID: "c", OutcomeID: "basic", Score: tc.basic})
			}
			session := diagnostic.Session{Status: diagnostic.StatusCompleted, Answers: answers, Variant: diagnostic.VariantSnapshot{IncludedCompetencyCount: 1}}
			got, err := New(scorePracticeRepo{}, scoreDiagnostics{session}, nil, nil, nil, nil, nil).Preview(context.Background(), "owner", "diagnostic")
			if err != nil || got.DiagnosticScore != tc.want || got.MaximumScore != 3 || got.Mode != tc.mode || len(got.Topics) != 1 {
				t.Fatalf("preview: %+v, %v", got, err)
			}
			if tc.main != 2 && (got.Topics[0].OriginalMaxScore == nil || *got.Topics[0].OriginalMaxScore != 3) {
				t.Fatalf("original scale: %+v", got.Topics[0])
			}
		})
	}
}
