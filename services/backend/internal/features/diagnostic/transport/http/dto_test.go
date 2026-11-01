package diagnostichttp

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func TestResultGradeUsesOnlyThePoints(t *testing.T) {
	for _, tc := range []struct {
		score, maximum int
		want           string
		percent        int
	}{
		// A partial main with two right basics is 3/3: points alone decide.
		{3, 3, "A", 100},
		{7, 9, "B", 77},
		{2, 3, "C", 66},
		{1, 3, "D", 33},
	} {
		r := resultResponse(diagnostic.Result{DiagnosticScore: tc.score, MaximumScore: tc.maximum})
		if r.Grade == nil || *r.Grade != tc.want || r.ScorePercent == nil || *r.ScorePercent != tc.percent {
			t.Errorf("%d/%d = %v %v", tc.score, tc.maximum, r.Grade, r.ScorePercent)
		}
	}
	if r := resultResponse(diagnostic.Result{}); r.Grade != nil || r.ScorePercent != nil {
		t.Fatal("a result without a maximum has no grade")
	}
}
