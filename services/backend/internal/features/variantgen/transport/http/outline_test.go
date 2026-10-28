package variantgenhttp

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
)

func TestOutlineAggregatesSelectionsForSameCompetency(t *testing.T) {
	comp := variant.CompetencyProfile{ID: "c", Name: "Competency"}
	makeTask := func(topic, outcome string) variant.VariantTask {
		return variant.VariantTask{Role: "main", Task: variant.TaskProfile{Constituent: variant.ConstituentProfile{ID: topic, Name: topic}, Outcome: variant.OutcomeProfile{ID: outcome, Name: outcome}}}
	}
	got := outlineDTO(variant.Variant{Competencies: []variant.CompetencySelection{
		{Competency: comp, Tasks: []variant.VariantTask{makeTask("a", "one")}},
		{Competency: comp, Tasks: []variant.VariantTask{makeTask("b", "two")}},
	}})
	if len(got.Competencies) != 1 || len(got.Competencies[0].Constituents) != 2 || got.Competencies[0].Constituents[1].Outcomes[0].ID != "two" {
		t.Fatalf("unexpected outline: %+v", got)
	}
}
