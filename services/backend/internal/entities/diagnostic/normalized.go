package diagnostic

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/scoring"

// ClosedCompetencyPercent is where a competency needs no more training.
const ClosedCompetencyPercent = 90

type competencyShare struct {
	id    string
	share float64
}

// competencyShares normalizes the answers: every asked task of a constituent
// weighs the same, as does every constituent of a competency, however many of
// them it took (decision of 01.11.2026).
func competencyShares(answers []Answer) []competencyShare {
	order := []string{}
	constituentOrder := map[string][]string{}
	tasks := map[string]map[string][]float64{}
	for _, a := range answers {
		if a.GraderMaxScore <= 0 {
			continue
		}
		if tasks[a.CompetencyID] == nil {
			tasks[a.CompetencyID] = map[string][]float64{}
			order = append(order, a.CompetencyID)
		}
		constituent := a.Task.ConstituentID
		if _, ok := tasks[a.CompetencyID][constituent]; !ok {
			constituentOrder[a.CompetencyID] = append(constituentOrder[a.CompetencyID], constituent)
		}
		tasks[a.CompetencyID][constituent] = append(tasks[a.CompetencyID][constituent], float64(a.Score)/float64(a.GraderMaxScore))
	}
	out := make([]competencyShare, 0, len(order))
	for _, id := range order {
		shares := make([]float64, 0, len(constituentOrder[id]))
		for _, constituent := range constituentOrder[id] {
			shares = append(shares, scoring.Mean(tasks[id][constituent]))
		}
		out = append(out, competencyShare{id: id, share: scoring.Mean(shares)})
	}
	return out
}

// OpenCompetencies are the competencies below ClosedCompetencyPercent: only
// they go into training.
func OpenCompetencies(answers []Answer) map[string]bool {
	open := map[string]bool{}
	for _, c := range competencyShares(answers) {
		if scoring.Percent(c.share) < ClosedCompetencyPercent {
			open[c.id] = true
		}
	}
	return open
}

// ReportPoints are the normalized points of the result: 5 per competency.
func (r Result) ReportPoints() (score float64, maximum int, percent *int) {
	competencies := competencyShares(r.Answers)
	shares := make([]float64, len(competencies))
	for i, c := range competencies {
		shares[i] = c.share
	}
	return scoring.Overall(shares)
}
