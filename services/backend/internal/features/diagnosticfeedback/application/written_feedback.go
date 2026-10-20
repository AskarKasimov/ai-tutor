package application

import (
	"fmt"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

// Model prose can replace presentation only; findings, scores and plan targets
// are still decided by AnalyzeResult and the accepted answer snapshots.
func applyWrittenFeedback(report *DeterministicReport, written WrittenFeedback) error {
	invalid := func() error {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель не сформировала полный разбор диагностики. Попробуйте ещё раз.")
	}
	if strings.TrimSpace(written.Summary) == "" {
		return invalid()
	}
	targets := map[string]*string{}
	for i := range report.Strengths {
		targets[fmt.Sprintf("strength:%d", i)] = &report.Strengths[i]
	}
	for i := range report.ConfirmedGaps {
		targets[fmt.Sprintf("gap:%d", i)] = &report.ConfirmedGaps[i].Advice
	}
	for i := range report.PartialCompetencies {
		targets[fmt.Sprintf("partial:%d", i)] = &report.PartialCompetencies[i].Details
	}
	if len(written.Details) != len(targets) {
		return invalid()
	}
	prose := map[string]string{}
	for _, detail := range written.Details {
		if targets[detail.ID] == nil || strings.TrimSpace(detail.Text) == "" {
			return invalid()
		}
		if _, duplicate := prose[detail.ID]; duplicate {
			return invalid()
		}
		prose[detail.ID] = strings.TrimSpace(detail.Text)
	}
	for _, evidence := range report.Evidence {
		text := prose[evidence.ID]
		if !strings.HasPrefix(evidence.ID, "strength:") {
			text = evidence.Prefix + text
		}
		*targets[evidence.ID] = text
		for i := range report.TrainingRecommendations {
			rec := &report.TrainingRecommendations[i]
			if rec.CompetencyID == evidence.CompetencyID && rec.OutcomeID == evidence.OutcomeID {
				rec.Rationale = text
			}
		}
	}
	return nil
}
