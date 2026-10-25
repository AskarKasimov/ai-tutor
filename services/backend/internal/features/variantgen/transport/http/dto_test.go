package variantgenhttp

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"testing"
)

func TestVariantDTOIncludesBlocksWithZeroOrOneBasic(t *testing.T) {
	for _, size := range []int{1, 2, 3} {
		value := variant.Variant{IncludedCompetencyCount: 1, TaskCount: size}
		block := variant.CompetencySelection{Tasks: []variant.VariantTask{{ID: "main", Role: "main"}}}
		for i := 1; i < size; i++ {
			block.Tasks = append(block.Tasks, variant.VariantTask{ID: "basic", Role: "basic"})
		}
		value.Competencies = []variant.CompetencySelection{block}
		got := variantDTO(value)
		if got.TaskCount != size || len(got.Competencies) != 1 || got.Competencies[0].Main.ID != "main" || len(got.Competencies[0].Basic) != size-1 || got.Competencies[0].Basic == nil {
			t.Fatalf("size %d: %+v", size, got)
		}
	}
}
