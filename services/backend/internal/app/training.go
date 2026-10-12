package app

import (
	"context"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	assessmentapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
)

type trainingGrader struct{ service *assessmentapp.Service }

func (g trainingGrader) EvaluateTraining(ctx context.Context, owner, transcriptionID string, e training.Exercise) (assessment.Evaluation, error) {
	return g.service.EvaluateTrusted(ctx, owner, transcriptionID, trainingVariantTask(e))
}

type trainingFinalGrader struct{ service *assessmentapp.Service }

func (g trainingFinalGrader) GradeFinal(ctx context.Context, exercise training.Exercise, answer string) (assessment.Evaluation, error) {
	return g.service.EvaluateTrustedText(ctx, trainingVariantTask(exercise), answer)
}

func trainingVariantTask(e training.Exercise) variant.VariantTask {
	t := e.Task
	instruction, reference, criteria := t.VoiceInstruction, t.ReferenceAnswer, t.Criteria
	taxonomy, level, content := t.TaxonomyCode, t.ALDLevelCode, t.EducationalContent
	return variant.VariantTask{ID: e.ID, Role: "training", Task: variant.TaskProfile{ID: t.SourceTaskID, Question: t.Question, Options: t.Options, VoiceInstruction: &instruction, ReferenceAnswer: &reference, Criteria: &criteria, Constituent: variant.ConstituentProfile{ID: t.ConstituentID, Name: t.ConstituentName}, Outcome: variant.OutcomeProfile{ID: t.OutcomeID, Name: t.OutcomeName, TaxonomyCode: &taxonomy, ALDLevelCode: &level, EducationalContent: &content}}}
}
