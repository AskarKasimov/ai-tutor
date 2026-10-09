package diagnostic

type TrainingTarget struct {
	Kind   string
	Answer Answer
}

// TrainingTargets uses only accepted answers, retaining diagnostic position order.
func TrainingTargets(answers []Answer) []TrainingTarget {
	main := map[string]Answer{}
	failed := map[string]bool{}
	for _, a := range answers {
		if a.Role == "main" {
			main[a.CompetencyID] = a
		}
		if a.Role == "basic" && a.Score == 0 {
			failed[a.CompetencyID] = true
		}
	}
	result := []TrainingTarget{}
	seen := map[string]bool{}
	for _, a := range answers {
		m, exists := main[a.CompetencyID]
		if !exists || m.Score == 2 {
			continue
		}
		kind := ""
		if failed[a.CompetencyID] && a.Role == "basic" && a.Score == 0 {
			kind = "confirmed_gap"
		}
		if !failed[a.CompetencyID] && a.Role == "main" {
			kind = "partial_competency"
		}
		outcomeID := a.OutcomeID
		if outcomeID == "" {
			outcomeID = a.Task.OutcomeID
		}
		if kind != "" && !seen[outcomeID] {
			result = append(result, TrainingTarget{Kind: kind, Answer: a})
			seen[outcomeID] = true
		}
	}
	return result
}
