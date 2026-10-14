package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func outcomeLabel(task diagnostic.TaskSnapshot) string {
	if strings.TrimSpace(task.ConstituentName) != "" {
		return fmt.Sprintf("Составляющая «%s», результат «%s»", task.ConstituentName, task.OutcomeName)
	}
	return fmt.Sprintf("Проверенный результат «%s»", task.OutcomeName)
}

func AnalyzeResult(result diagnostic.Result) DeterministicReport {
	report := DeterministicReport{SessionID: result.SessionID, DiagnosticScore: result.DiagnosticScore,
		MaximumScore: result.MaximumScore, Strengths: []string{}, ConfirmedGaps: []diagnostic.ConfirmedGap{},
		PartialCompetencies:     []diagnostic.PartialCompetency{},
		UnverifiedCompetencies:  append([]diagnostic.SkippedCompetency(nil), result.SkippedCompetencies...),
		TrainingRecommendations: []diagnostic.TrainingRecommendation{}}
	if result.MaximumScore > 0 {
		report.ScorePercentage = result.DiagnosticScore * 100 / result.MaximumScore
	}
	for _, finding := range diagnostic.OutcomeFindings(result.Answers) {
		a := finding.Answer
		label := outcomeLabel(a.Task)
		if finding.Mastered {
			report.Strengths = append(report.Strengths, label+": продемонстрированы хорошие знания по проверенному заданию.")
			continue
		}
		advice := "Дополнительно изучить. " + label + "."
		if finding.FullStudy {
			advice = "Необходимо изучить полностью составляющую «" + a.Task.ConstituentName + "». Проверенный результат: «" + a.Task.OutcomeName + "»."
		}
		if a.Skipped {
			advice = "Вопрос пропущен: необходимо проверить и отработать результат. " + label + "."
		}
		if len(a.Feedback) >= 3 && !a.Skipped {
			next := cleanSentence(a.Feedback[2])
			if next != "" && !strings.Contains(strings.ToLower(next), "следующ") {
				advice += " " + next + "."
			}
		}
		failed := []string{}
		for _, cr := range a.CriterionResults {
			if !cr.Satisfied && strings.TrimSpace(cr.Explanation) != "" {
				failed = append(failed, cr.Explanation)
			}
		}
		id := a.Task.OutcomeID
		if id == "" {
			id = a.OutcomeID
		}
		// Keep the existing wire shape: partial main answers use details;
		// zero answers use gaps. Both carry an explicit result and constituent.
		if a.Role == "main" && a.Score == 1 && !a.Skipped {
			report.PartialCompetencies = append(report.PartialCompetencies, diagnostic.PartialCompetency{
				CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName, Details: advice})
		} else {
			report.ConfirmedGaps = append(report.ConfirmedGaps, diagnostic.ConfirmedGap{
				CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName, OutcomeID: id,
				OutcomeName: a.Task.OutcomeName, TaxonomyCode: a.Task.TaxonomyCode, Importance: a.Task.Importance,
				FailedCriteria: failed, Advice: advice})
		}
		report.TrainingRecommendations = append(report.TrainingRecommendations, diagnostic.TrainingRecommendation{
			CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName,
			OutcomeID: id, OutcomeName: a.Task.OutcomeName, Rationale: advice})
	}
	// Legacy snapshots keep the outcome ID on the answer, as the items above.
	importance := map[string]int16{}
	for _, a := range result.Answers {
		id := a.Task.OutcomeID
		if id == "" {
			id = a.OutcomeID
		}
		importance[id] = a.Task.Importance
	}
	sort.SliceStable(report.TrainingRecommendations, func(i, j int) bool {
		return importance[report.TrainingRecommendations[i].OutcomeID] > importance[report.TrainingRecommendations[j].OutcomeID]
	})
	for i := range report.TrainingRecommendations {
		report.TrainingRecommendations[i].Priority = i + 1
	}
	return report
}
