package diagnostic

type TrainingTarget struct {
	Kind   string
	Answer Answer
}

// TrainingTargets uses only accepted answers, retaining diagnostic position order.
func TrainingTargets(answers []Answer) []TrainingTarget {
	return trainingTargets(answers, false)
}

func trainingTargets(answers []Answer, byConstituent bool) []TrainingTarget {
	key := func(a Answer) string {
		if byConstituent {
			return a.CompetencyID + "\x00" + a.Task.ConstituentID
		}
		return a.CompetencyID
	}
	main := map[string]Answer{}
	failed := map[string]bool{}
	for _, a := range answers {
		if a.Role == "main" {
			main[key(a)] = a
		}
		if a.Role == "basic" && a.Score == 0 {
			failed[key(a)] = true
		}
	}
	result := []TrainingTarget{}
	seen := map[string]bool{}
	for _, a := range answers {
		m, exists := main[key(a)]
		if !exists || m.Score >= 2 {
			continue
		}
		kind := ""
		if failed[key(a)] && a.Role == "basic" && a.Score == 0 {
			kind = "confirmed_gap"
		}
		if !failed[key(a)] && a.Role == "main" {
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

func TrainingTargetsByConstituent(answers []Answer) []TrainingTarget {
	return trainingTargets(answers, true)
}
