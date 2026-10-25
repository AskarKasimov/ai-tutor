package application

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
)

func TestSnapshotVariantPreservesAudioAssetReference(t *testing.T) {
	instruction, answer, assetID := "Скажите ответ", "ответ", "taskaudio_source-task"
	value, err := snapshotVariant(variant.Variant{
		ID: "variant-1", IncludedCompetencyCount: 1,
		Competencies: []variant.CompetencySelection{{Position: 1, Competency: variant.CompetencyProfile{ID: "competency-1", Name: "Компетенция"}, Tasks: []variant.VariantTask{{
			ID: "variant-task-1", Role: "main", Task: variant.TaskProfile{
				ID: "source-task", AudioAssetID: &assetID, Question: "Вопрос", Options: []string{},
				VoiceInstruction: &instruction, ReferenceAnswer: &answer,
				Competency:  variant.CompetencyProfile{ID: "competency-1", Name: "Компетенция"},
				Constituent: variant.ConstituentProfile{ID: "constituent-1", Name: "Составляющая"},
				Outcome:     variant.OutcomeProfile{ID: "outcome-1", Name: "Результат"},
			},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := value.Competencies[0].Tasks[0].AudioAssetID
	if got == nil || *got != assetID {
		t.Fatalf("diagnostic snapshot audio asset = %v", got)
	}
}
