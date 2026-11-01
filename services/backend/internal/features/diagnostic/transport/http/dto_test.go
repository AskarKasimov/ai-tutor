package diagnostichttp

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func TestResultNormalizesEveryCompetency(t *testing.T) {
	task := func(competency, constituent, role string, score int) diagnostic.Answer {
		maximum := 1
		if role == "main" {
			maximum = 3
		}
		return diagnostic.Answer{CompetencyID: competency, Role: role, Score: score, GraderMaxScore: maximum,
			Task: diagnostic.TaskSnapshot{ConstituentID: constituent}}
	}
	main, basic := "main", "basic"
	for _, tc := range []struct {
		name     string
		answers  []diagnostic.Answer
		score    float64
		maximum  int
		want     string
		percent  int
		training bool
	}{
		{"full main", []diagnostic.Answer{task("a", "1", main, 3)}, 5, 5, "A", 100, false},
		{"partial main and both basics", []diagnostic.Answer{task("a", "1", main, 1), task("a", "1", basic, 1), task("a", "1", basic, 1)}, 3.9, 5, "B", 77, true},
		{"failed main and both basics", []diagnostic.Answer{task("a", "1", main, 0), task("a", "1", basic, 1), task("a", "1", basic, 1)}, 3.3, 5, "C", 66, true},
		{"partial main and one basic", []diagnostic.Answer{task("a", "1", main, 1), task("a", "1", basic, 1), task("a", "1", basic, 0)}, 2.2, 5, "D", 44, true},
		{"two competencies", []diagnostic.Answer{task("a", "1", main, 3), task("b", "1", main, 1), task("b", "1", basic, 1), task("b", "1", basic, 1)}, 8.9, 10, "B", 88, true},
		{"more constituents do not weigh more", []diagnostic.Answer{task("a", "1", main, 3), task("a", "2", main, 3), task("a", "3", main, 3), task("b", "1", main, 0), task("b", "1", basic, 0), task("b", "1", basic, 0)}, 5, 10, "D", 50, true},
		{"a constituent averages inside its competency", []diagnostic.Answer{task("a", "1", main, 3), task("a", "2", main, 0), task("a", "2", basic, 1), task("a", "2", basic, 1)}, 4.2, 5, "B", 83, true},
	} {
		// The stored total keeps its own scale for the plan and training.
		r := resultResponse(diagnostic.Result{DiagnosticScore: 3, MaximumScore: 3, Answers: tc.answers})
		if r.ReportScore != tc.score || r.ReportMaxScore != tc.maximum || r.Grade == nil || *r.Grade != tc.want || *r.ScorePercent != tc.percent || r.TrainingNeeded != tc.training {
			t.Errorf("%s = %v/%d %v %v training %v", tc.name, r.ReportScore, r.ReportMaxScore, r.Grade, r.ScorePercent, r.TrainingNeeded)
		}
		if r.DiagnosticScore != 3 || r.MaximumScore != 3 {
			t.Errorf("%s changed the stored total: %d/%d", tc.name, r.DiagnosticScore, r.MaximumScore)
		}
	}
	if r := resultResponse(diagnostic.Result{MaximumScore: 3}); r.Grade != nil || r.ScorePercent != nil || r.TrainingNeeded {
		t.Fatal("a result without asked tasks has no grade")
	}
}
