package application

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
	"reflect"
	"testing"
)

func TestPlanUsesCheckedGapsAndOnlyFullyUnstudiedConstituent(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		scores                   []int
		mixed, skipped, reimport bool
		want                     []string
	}{
		{"bases mastered", []int{0, 1, 1}, false, false, false, []string{"main"}},
		{"partial main and bases mastered", []int{1, 1, 1}, false, false, false, []string{"main"}},
		{"one failed base", []int{1, 0, 1}, false, false, false, []string{"main", "b1"}},
		{"whole constituent", []int{0, 0, 0}, false, false, false, []string{"main", "b1", "b2", "extra"}},
		{"partial main not whole", []int{1, 0, 0}, false, false, false, []string{"main", "b1", "b2"}},
		{"mixed constituents", []int{0, 0, 0}, true, false, false, []string{"main", "b1", "b2"}},
		{"skipped not proven whole", []int{0, 0, 0}, false, true, false, []string{"main", "b1", "b2"}},
		{"reimport uses snapshot", []int{0, 0, 0}, false, false, true, []string{"main", "b1", "b2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, _, _ := newService(t, 1)
			session := diagnostic.Session{Variant: diagnostic.VariantSnapshot{MapRevision: 7}}
			for i, id := range []string{"main", "b1", "b2"} {
				role := "basic"
				if i == 0 {
					role = "main"
				}
				part := "part"
				if tc.mixed && i == 2 {
					part = "other"
				}
				session.Answers = append(session.Answers, diagnostic.Answer{CompetencyID: "c1", Role: role, Score: tc.scores[i], Skipped: tc.skipped && i == 2,
					Task: diagnostic.TaskSnapshot{OutcomeID: id, OutcomeName: "snapshot " + id, ConstituentID: part, ConstituentName: part}})
			}
			repo.outcomes = nil
			revision := int64(7)
			if tc.reimport {
				revision = 8
			}
			for _, id := range []string{"main", "b1", "b2", "extra", "unrelated"} {
				part := "part"
				if id == "unrelated" || tc.mixed && id == "b2" {
					part = "other"
				}
				repo.outcomes = append(repo.outcomes, learning.Outcome{CompetencyID: "c1", ConstituentID: part, MapRevision: revision, OutcomeID: id, OutcomeName: "map " + id})
			}
			outcomes, err := s.outcomes(t.Context(), session)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, o := range outcomes {
				got = append(got, o.OutcomeID)
				if tc.reimport && o.OutcomeName != "snapshot "+o.OutcomeID {
					t.Fatal("historical snapshot replaced by current map")
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
