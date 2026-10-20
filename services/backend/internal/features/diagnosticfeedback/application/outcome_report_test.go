package application

import (
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func TestReportDoesNotAppendTaskActions(t *testing.T) {
	for _, answer := range []struct {
		name  string
		role  string
		score int
	}{
		{"incorrect main", "main", 0},
		{"partial main", "main", 1},
		{"incorrect basic", "basic", 0},
	} {
		for _, next := range []string{
			"Выберите правильный ответ.",
			"Назовите вариант словами и объясните свой выбор.",
			"Повторите типы задач. Выберите один из предложенных вариантов.",
		} {
			t.Run(answer.name+"/"+next, func(t *testing.T) {
				result := diagnostic.Result{Answers: []diagnostic.Answer{{
					CompetencyID: "c", Role: answer.role, Score: answer.score,
					Feedback: []string{"Ответ неполный.", "Не указан тип целевой переменной.", next},
					Task: diagnostic.TaskSnapshot{
						OutcomeID: "o", OutcomeName: "Различает классификацию и регрессию",
						ConstituentID: "part", ConstituentName: "Постановка задачи",
					},
				}}}
				report := AnalyzeResult(result)
				want := fallbackTopicText("Постановка задачи", StudyReview)
				if answer.score == 1 {
					if len(report.PartialCompetencies) != 1 || report.PartialCompetencies[0].Details != want {
						t.Fatalf("partial advice contains task actions: %+v", report.PartialCompetencies)
					}
				} else if len(report.ConfirmedGaps) != 1 || report.ConfirmedGaps[0].Advice != want {
					t.Fatalf("gap advice contains task actions: %+v", report.ConfirmedGaps)
				}
				if len(report.TrainingRecommendations) != 1 || report.TrainingRecommendations[0].Rationale != want {
					t.Fatalf("training rationale contains task actions: %+v", report.TrainingRecommendations)
				}
				if result.Answers[0].Feedback[2] != next {
					t.Fatal("original answer feedback was changed")
				}
			})
		}
	}
}

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
				if (gap.StudyScope == StudyFull) != tc.full || strings.Contains(gap.Advice, "изучить полностью") != tc.full {
					t.Fatalf("wrong scope: %+v", gap)
				}
			}
			for _, strength := range r.Strengths {
				if strings.Contains(strength, "высокий уровень владения") || !strings.Contains(strength, "проверенное задание") {
					t.Fatalf("overbroad strength: %s", strength)
				}
			}
			if tc.name == "mastered basics" && (len(r.Strengths) != 2 || len(r.TrainingRecommendations) != 1 || r.TrainingRecommendations[0].OutcomeID != "main") {
				t.Fatalf("mastered basics treated as gaps: %+v", r)
			}
		})
	}
}

func TestHistoricalFeedbackIsUpgradedOnce(t *testing.T) {
	for _, version := range []int{0, 2, 3, 4, 5} {
		store := &mockStore{data: map[string]diagnostic.OverallFeedback{"u/s": {SessionID: "s", AnalysisVersion: version, Summary: "Выберите правильный ответ."}}}
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
}

type assertionError struct{}

func (assertionError) Error() string { return "must use cache" }
