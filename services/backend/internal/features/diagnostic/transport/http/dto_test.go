package diagnostichttp

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func TestResultGradeWeighsEveryAskedTaskTheSame(t *testing.T) {
	main := func(score int) diagnostic.Answer {
		return diagnostic.Answer{Role: "main", Score: score, GraderMaxScore: 3}
	}
	basic := func(score int) diagnostic.Answer {
		return diagnostic.Answer{Role: "basic", Score: score, GraderMaxScore: 1}
	}
	for _, tc := range []struct {
		name           string
		answers        []diagnostic.Answer
		score, maximum int
		want           string
		percent        int
	}{
		{"full main credits its basics", []diagnostic.Answer{main(3)}, 3, 3, "A", 100},
		{"partial main and both basics", []diagnostic.Answer{main(1), basic(1), basic(1)}, 7, 9, "B", 77},
		{"failed main and both basics", []diagnostic.Answer{main(0), basic(1), basic(1)}, 6, 9, "C", 66},
		{"partial main and one basic", []diagnostic.Answer{main(1), basic(1), basic(0)}, 4, 9, "D", 44},
		{"failed main and one basic", []diagnostic.Answer{main(0), basic(1), basic(0)}, 3, 9, "D", 33},
		{"two competencies", []diagnostic.Answer{main(3), main(1), basic(1), basic(1)}, 10, 12, "B", 83},
	} {
		// The stored total keeps its own scale: 3 per competency.
		r := resultResponse(diagnostic.Result{DiagnosticScore: 3, MaximumScore: 3, Answers: tc.answers})
		if r.ReportScore != tc.score || r.ReportMaxScore != tc.maximum || r.Grade == nil || *r.Grade != tc.want || *r.ScorePercent != tc.percent {
			t.Errorf("%s = %d/%d %v %v", tc.name, r.ReportScore, r.ReportMaxScore, r.Grade, r.ScorePercent)
		}
		if r.DiagnosticScore != 3 || r.MaximumScore != 3 {
			t.Errorf("%s changed the stored total: %d/%d", tc.name, r.DiagnosticScore, r.MaximumScore)
		}
	}
	r := resultResponse(diagnostic.Result{Answers: []diagnostic.Answer{basic(1)}})
	if r.Answers[0].ReportScore != 3 || r.Answers[0].ReportMaxScore != 3 || r.Answers[0].Score != 1 {
		t.Fatalf("basic answer = %+v", r.Answers[0])
	}
	if r := resultResponse(diagnostic.Result{MaximumScore: 3}); r.Grade != nil || r.ScorePercent != nil {
		t.Fatal("a result without asked tasks has no grade")
	}
}
