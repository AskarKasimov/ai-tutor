package application

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"strings"
	"testing"
)

func TestReportLimitsClaimsToCheckedOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores []int
		mixed  bool
		full   bool
	}{
		{"all zero same constituent", []int{0, 0, 0}, false, true},
		{"all zero mixed", []int{0, 0, 0}, true, false},
		{"partial main", []int{1, 0, 0}, false, false},
		{"mastered basics", []int{0, 1, 1}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answers := []diagnostic.Answer{}
			for i, id := range []string{"main", "b1", "b2"} {
				role := "basic"
				if i == 0 {
					role = "main"
				}
				part := "part"
				if tc.mixed && i == 2 {
					part = "other"
				}
				answers = append(answers, diagnostic.Answer{CompetencyID: "c", Role: role, Score: tc.scores[i], Task: diagnostic.TaskSnapshot{OutcomeID: id, OutcomeName: id, ConstituentID: part, ConstituentName: part, CompetencyName: "broad competency"}})
			}
			r := AnalyzeResult(diagnostic.Result{Answers: answers})
			for _, gap := range r.ConfirmedGaps {
				if strings.Contains(gap.Advice, "изучить полностью") != tc.full {
					t.Fatalf("wrong scope: %+v", gap)
				}
			}
			for _, strength := range r.Strengths {
				if strings.Contains(strength, "высокий уровень владения") || !strings.Contains(strength, "результат") {
					t.Fatalf("overbroad strength: %s", strength)
				}
			}
			if tc.name == "mastered basics" && (len(r.Strengths) != 2 || len(r.TrainingRecommendations) != 1 || r.TrainingRecommendations[0].OutcomeID != "main") {
				t.Fatalf("mastered basics treated as gaps: %+v", r)
			}
		})
	}
}

func TestReportKeepsTheReasonOfAWrongAnswer(t *testing.T) {
	const reason = "Не объяснено различие L1 и L2"
	failed := []diagnostic.CriterionResult{{Key: "justification", Explanation: reason}}
	task := diagnostic.TaskSnapshot{CompetencyName: "Регуляризация", OutcomeID: "main", OutcomeName: "L1 и L2"}
	r := AnalyzeResult(diagnostic.Result{Answers: []diagnostic.Answer{
		{CompetencyID: "c", Role: "main", Score: 1, CriterionResults: failed, Task: task},
	}})
	if len(r.PartialCompetencies) != 1 || !strings.Contains(r.PartialCompetencies[0].Details, reason+".") {
		t.Fatalf("partial answer lost its reason: %+v", r.PartialCompetencies)
	}
	if len(r.TrainingRecommendations) != 1 || !strings.Contains(r.TrainingRecommendations[0].Rationale, reason+".") {
		t.Fatalf("recommendation lost the reason: %+v", r.TrainingRecommendations)
	}

	r = AnalyzeResult(diagnostic.Result{Answers: []diagnostic.Answer{
		{CompetencyID: "c", Role: "main", Skipped: true, CriterionResults: []diagnostic.CriterionResult{{Key: "skipped", Explanation: "Вопрос пропущен."}}, Task: task},
	}})
	if len(r.TrainingRecommendations) != 1 || strings.Contains(r.TrainingRecommendations[0].Rationale, "Вопрос пропущен.") {
		t.Fatalf("skipped answer repeats its criterion: %+v", r.TrainingRecommendations)
	}
}

func TestHistoricalFeedbackIsUpgradedOnce(t *testing.T) {
	store := &mockStore{data: map[string]diagnostic.OverallFeedback{"u/s": {SessionID: "s", Summary: "old overbroad summary"}}}
	synth := &mockSynthesizer{summary: "new scoped summary"}
	svc := New(&mockReader{result: diagnostic.Result{SessionID: "s"}}, synth, store, nil)
	got, err := svc.GetFeedback(t.Context(), "u", "s")
	if err != nil || got.Summary != "new scoped summary" || got.AnalysisVersion != analysisVersion {
		t.Fatalf("upgrade: %+v %v", got, err)
	}
	synth.err = assertionError{}
	if _, err := svc.GetFeedback(t.Context(), "u", "s"); err != nil {
		t.Fatalf("same version regenerated: %v", err)
	}
}

type assertionError struct{}

func (assertionError) Error() string { return "must use cache" }
