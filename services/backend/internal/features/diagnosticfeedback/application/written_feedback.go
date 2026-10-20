package application

import (
	"strconv"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
)

// applyWrittenFeedback places the model prose that passes the checks; the rest
// keeps its current text. Findings, scores and plan targets stay as decided by
// AnalyzeResult. It reports whether every finding and the summary were written.
func applyWrittenFeedback(report *DeterministicReport, written WrittenFeedback) bool {
	complete := true
	if summary := strings.TrimSpace(written.Summary); assessment.Narrative(summary) {
		report.Summary = summary
	} else {
		complete = false
	}
	prose := map[string]string{}
	seen := map[string]bool{}
	for _, detail := range written.Details {
		text := strings.TrimSpace(detail.Text)
		if seen[detail.ID] {
			delete(prose, detail.ID)
			continue
		}
		seen[detail.ID] = true
		if assessment.Narrative(text) {
			prose[detail.ID] = text
		}
	}
	for _, evidence := range report.Evidence {
		text, ok := prose[evidence.ID]
		if !ok {
			complete = false
			continue
		}
		if index, found := strings.CutPrefix(evidence.ID, "strength:"); found {
			if i, err := strconv.Atoi(index); err == nil && i >= 0 && i < len(report.Strengths) {
				report.Strengths[i] = text
			}
			continue
		}
		setTopicText(report, evidence, text)
	}
	return complete
}
