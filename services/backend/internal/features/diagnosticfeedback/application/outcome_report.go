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
		evidence := FeedbackEvidence{Topic: a.Task.ConstituentName, Outcome: a.Task.OutcomeName,
			FullStudy: finding.FullStudy, Skipped: a.Skipped, CompetencyID: a.CompetencyID,
			OutcomeID: a.Task.OutcomeID, Observations: []string{}, Criteria: a.CriterionResults}
		if evidence.OutcomeID == "" {
			evidence.OutcomeID = a.OutcomeID
		}
		// Use the observed answer and clarification, not the closing recommendation.
		if !a.Skipped {
			evidence.Observations = append(evidence.Observations, a.Feedback[:min(2, len(a.Feedback))]...)
		}
		if finding.Mastered {
			evidence.ID = fmt.Sprintf("strength:%d", len(report.Strengths))
			report.Evidence = append(report.Evidence, evidence)
			report.Strengths = append(report.Strengths, label+": продемонстрированы хорошие знания по проверенному заданию.")
			continue
		}
		// Per-answer feedback may ask the student to select or explain an answer.
		// The completed diagnostic recommends outcomes to study, not task actions.
		advice := "Дополнительно изучить. " + label + "."
		if finding.FullStudy {
			advice = "Необходимо изучить полностью составляющую «" + a.Task.ConstituentName + "». Проверенный результат: «" + a.Task.OutcomeName + "»."
		}
		if a.Skipped {
			advice = "Вопрос пропущен: необходимо проверить и отработать результат. " + label + "."
		}
		if strings.TrimSpace(a.Task.ConstituentName) != "" {
			evidence.Prefix = "Дополнительно изучить. Составляющая «" + a.Task.ConstituentName + "», "
			if finding.FullStudy {
				evidence.Prefix = "Необходимо изучить полностью составляющую «" + a.Task.ConstituentName + "». "
			}
			if a.Skipped {
				evidence.Prefix = "Вопрос пропущен: необходимо проверить и отработать результат. Составляющая «" + a.Task.ConstituentName + "», "
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
			evidence.ID = fmt.Sprintf("partial:%d", len(report.PartialCompetencies))
			report.PartialCompetencies = append(report.PartialCompetencies, diagnostic.PartialCompetency{
				CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName, Details: advice})
		} else {
			evidence.ID = fmt.Sprintf("gap:%d", len(report.ConfirmedGaps))
			report.ConfirmedGaps = append(report.ConfirmedGaps, diagnostic.ConfirmedGap{
				CompetencyID: a.CompetencyID, CompetencyName: a.Task.CompetencyName, OutcomeID: id,
				OutcomeName: a.Task.OutcomeName, TaxonomyCode: a.Task.TaxonomyCode, Importance: a.Task.Importance,
				FailedCriteria: failed, Advice: advice})
		}
		report.Evidence = append(report.Evidence, evidence)
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
