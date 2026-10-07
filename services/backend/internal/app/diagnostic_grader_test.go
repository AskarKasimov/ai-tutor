package app

import (
	"context"
	"testing"

	assessmentapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
)

type diagnosticAssessorStub struct {
	ownerID, transcriptionID, variantID, variantTaskID string
	result                                             assessmentapp.Evaluation
}

func (s *diagnosticAssessorStub) EvaluateVariant(_ context.Context, ownerID, transcriptionID, variantID, variantTaskID string) (assessmentapp.Evaluation, error) {
	s.ownerID, s.transcriptionID, s.variantID, s.variantTaskID = ownerID, transcriptionID, variantID, variantTaskID
	return s.result, nil
}

func TestDiagnosticAssessmentGraderUsesVariantPositionAndPreservesRoleScale(t *testing.T) {
	assessor := &diagnosticAssessorStub{result: assessmentapp.Evaluation{
		Score: 1, MaxScore: 1, Verdict: "correct",
		CriterionResults: []assessmentapp.CriterionResult{{Key: "accuracy", Satisfied: true, Explanation: "Ответ точный."}},
		Feedback:         []string{"Верно.", "Критерии выполнены.", "Продолжайте."},
	}}
	grader := diagnosticAssessmentGrader{service: assessor}
	got, err := grader.Evaluate(context.Background(), "owner-1", "transcription-1", "variant-1", "variant-task-basic")
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if assessor.ownerID != "owner-1" || assessor.transcriptionID != "transcription-1" || assessor.variantID != "variant-1" || assessor.variantTaskID != "variant-task-basic" {
		t.Fatalf("EvaluateVariant args = (%q, %q, %q, %q)", assessor.ownerID, assessor.transcriptionID, assessor.variantID, assessor.variantTaskID)
	}
	if got.Score != 1 || got.MaxScore != 1 || got.Verdict != "correct" || len(got.CriterionResults) != 1 || !got.CriterionResults[0].Satisfied || len(got.Feedback) != 3 {
		t.Fatalf("assessment result lost during adapter conversion: %+v", got)
	}
	got.Feedback[0] = "mutated"
	if assessor.result.Feedback[0] == "mutated" {
		t.Fatal("adapter returned feedback backed by assessor-owned slice")
	}
}
