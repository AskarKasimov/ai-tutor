package exam

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
)

func fixture(t *testing.T) Graph {
	t.Helper()
	data, err := os.ReadFile("../../../../../api/fixtures/course-graph.json")
	if err != nil {
		t.Fatal(err)
	}
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	return g
}

var (
	strong  = Evaluation{Verdict: VerdictAnswered, Criteria: Criteria{2, 2, 2, 2}}
	partial = Evaluation{Verdict: VerdictAnswered, Criteria: Criteria{2, 2, 1, 1}}
	weak    = Evaluation{Verdict: VerdictAnswered, Criteria: Criteria{0, 1, 0, 0}}
	unknown = Evaluation{Verdict: VerdictDontKnow}
	vague   = Evaluation{Verdict: VerdictInsufficient}
)

// run answers every step with answer(step) and returns the asked steps.
func run(t *testing.T, e *Exam, first Step, answer func(Step) Evaluation) []Step {
	t.Helper()
	steps := []Step{}
	progress := 0
	for step := first; step.Kind != KindClosing; {
		steps = append(steps, step)
		if len(steps) > 100 {
			t.Fatal("exam does not end")
		}
		step = e.Apply(answer(step))
		if p := e.Progress(); p < progress {
			t.Fatalf("progress went down: %d -> %d", progress, p)
		} else {
			progress = p
		}
		if !e.Completed() && progress >= 100 {
			t.Fatal("progress reached 100 before completion")
		}
	}
	if e.Progress() != 100 {
		t.Fatalf("completed exam progress = %d", e.Progress())
	}
	return steps
}

func asked(steps []Step, kind TurnKind) []string {
	out := []string{}
	for _, s := range steps {
		if s.Kind == kind {
			out = append(out, s.NodeID)
		}
	}
	return out
}

func TestTopLevelAndClosure(t *testing.T) {
	g := fixture(t)
	if got := g.TopLevel(); !slices.Equal(got, []string{"a-star", "simulated-annealing", "mcts", "agents-peas"}) {
		t.Fatalf("top level = %v", got)
	}
	closure := g.Closure("a-star")
	slices.Sort(closure)
	if want := []string{"heuristic", "heuristic-admissibility", "search-space", "uninformed-search"}; !slices.Equal(closure, want) {
		t.Fatalf("closure = %v", closure)
	}
}

func TestStrongStudentIsAskedOnlyTopLevelTopics(t *testing.T) {
	e, first := NewExam(fixture(t), 0)
	steps := run(t, e, first, func(s Step) Evaluation {
		if s.Kind == KindFollowup {
			return strong
		}
		return partial
	})
	if got := asked(steps, KindQuestion); !slices.Equal(got, []string{"a-star", "simulated-annealing", "mcts", "agents-peas"}) {
		t.Fatalf("questions = %v", got)
	}
	if got := asked(steps, KindFollowup); len(got) != 4 {
		t.Fatalf("followups = %v", got)
	}
	for _, id := range []string{"heuristic", "search-space", "hill-climbing", "exploration-exploitation"} {
		if e.Nodes[id].Status != StatusCredited {
			t.Fatalf("%s status = %s", id, e.Nodes[id].Status)
		}
	}
	grade, percent := e.CourseGrade()
	if *grade != GradeA || *percent != 100 || len(e.Gaps()) != 0 {
		t.Fatalf("grade=%s percent=%d gaps=%v", *grade, *percent, e.Gaps())
	}
}

func TestGapDescendsToTheDeepestUnmasteredTopic(t *testing.T) {
	e, first := NewExam(fixture(t), 0)
	steps := run(t, e, first, func(s Step) Evaluation {
		switch s.NodeID {
		case "a-star":
			return weak
		case "heuristic-admissibility":
			return unknown
		}
		if s.Kind == KindFollowup {
			return strong
		}
		return partial
	})
	// a-star is weak twice (with a rephrase), then its prerequisites are asked
	// before the next top-level topic.
	if got := asked(steps, KindQuestion); !slices.Equal(got[:4], []string{"a-star", "uninformed-search", "heuristic-admissibility", "heuristic"}) {
		t.Fatalf("questions = %v", got)
	}
	if got := asked(steps, KindRephrase); !slices.Equal(got, []string{"a-star"}) {
		t.Fatalf("rephrases = %v (don't know is never rephrased)", got)
	}
	if e.Nodes["a-star"].Status != StatusGap || !e.Nodes["a-star"].Assisted || e.Nodes["heuristic-admissibility"].Status != StatusGap {
		t.Fatalf("a-star=%+v admissibility=%+v", e.Nodes["a-star"], e.Nodes["heuristic-admissibility"])
	}
	if e.Nodes["search-space"].Status != StatusCredited {
		t.Fatalf("search-space = %s", e.Nodes["search-space"].Status)
	}
	if got := e.Gaps(); !slices.Equal(got, []string{"heuristic-admissibility"}) {
		t.Fatalf("gaps = %v", got)
	}
	// Top level: D, A, A, A -> 13/16 = 81%.
	grade, percent := e.CourseGrade()
	if *grade != GradeB || *percent != 81 {
		t.Fatalf("grade=%s percent=%d", *grade, *percent)
	}
}

func TestCreditedSharedPrerequisiteIsNotAskedAgain(t *testing.T) {
	e, first := NewExam(fixture(t), 0)
	steps := run(t, e, first, func(s Step) Evaluation {
		if s.NodeID == "simulated-annealing" {
			return unknown
		}
		if s.Kind == KindFollowup {
			return strong
		}
		return partial
	})
	// a-star credits heuristic; the gap on simulated-annealing then asks only
	// hill-climbing and exploration-exploitation, not heuristic again.
	questions := asked(steps, KindQuestion)
	if slices.Contains(questions, "heuristic") {
		t.Fatalf("credited prerequisite asked again: %v", questions)
	}
	if !slices.Equal(questions[:4], []string{"a-star", "simulated-annealing", "hill-climbing", "exploration-exploitation"}) {
		t.Fatalf("questions = %v", questions)
	}
	if got := e.Gaps(); !slices.Equal(got, []string{"simulated-annealing"}) {
		t.Fatalf("gaps = %v", got)
	}
}

func TestAskedTopicKeepsItsOwnResult(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: "base", Title: "База"},
		{ID: "left", Title: "Левая", Prerequisites: []string{"base"}},
		{ID: "right", Title: "Правая", Prerequisites: []string{"base"}},
	}}
	e, first := NewExam(g, 0)
	run(t, e, first, func(s Step) Evaluation {
		switch {
		case s.NodeID == "left", s.NodeID == "base":
			return unknown
		case s.Kind == KindFollowup:
			return strong
		}
		return partial
	})
	// base was asked under left and stays a gap although right later passes.
	if e.Nodes["base"].Status != StatusGap || e.Nodes["right"].Status != StatusIndependent {
		t.Fatalf("base=%s right=%s", e.Nodes["base"].Status, e.Nodes["right"].Status)
	}
}

func TestAnswerAfterRephraseIsAssistedAndCappedAtB(t *testing.T) {
	e, first := NewExam(fixture(t), 0)
	run(t, e, first, func(s Step) Evaluation {
		if s.NodeID == "agents-peas" && s.Kind == KindQuestion {
			return weak
		}
		return strong
	})
	state := e.Nodes["agents-peas"]
	if state.Status != StatusAssisted || *state.Grade != GradeB {
		t.Fatalf("agents-peas = %+v grade=%s", state, *state.Grade)
	}
}

func TestInsufficientAfterRephraseIsExcludedFromTheGrade(t *testing.T) {
	e, first := NewExam(fixture(t), 0)
	steps := run(t, e, first, func(s Step) Evaluation {
		if s.NodeID == "mcts" && s.Kind != KindFollowup {
			return vague
		}
		return strong
	})
	if got := asked(steps, KindRephrase); !slices.Equal(got, []string{"mcts"}) {
		t.Fatalf("rephrases = %v", got)
	}
	if e.Nodes["mcts"].Status != StatusInsufficient || e.Nodes["mcts"].Grade != nil {
		t.Fatalf("mcts = %+v", e.Nodes["mcts"])
	}
	if e.Nodes["exploration-exploitation"].Status != StatusCredited {
		t.Fatalf("exploration credited by simulated-annealing: %s", e.Nodes["exploration-exploitation"].Status)
	}
	grade, percent := e.CourseGrade()
	if *grade != GradeA || *percent != 100 {
		t.Fatalf("grade=%s percent=%d", *grade, *percent)
	}
}

func TestMaxQuestionsEndsTheExam(t *testing.T) {
	e, first := NewExam(fixture(t), 3)
	steps := run(t, e, first, func(s Step) Evaluation { return strong })
	// a-star: question + followup, simulated-annealing: question + followup
	// would be four, so the exam closes after the second followup is due.
	if q := len(asked(steps, KindQuestion)) + len(asked(steps, KindFollowup)); q > 4 {
		t.Fatalf("asked %d times with a limit of 3", q)
	}
	if e.Nodes["agents-peas"].Status != StatusUnchecked {
		t.Fatal("topics beyond the limit stay unchecked")
	}
}

func TestValidateRejectsCyclesAndUnknownPrerequisites(t *testing.T) {
	cycle := Graph{Nodes: []Node{{ID: "a", Title: "A", Prerequisites: []string{"b"}}, {ID: "b", Title: "B", Prerequisites: []string{"a"}}}}
	unknown := Graph{Nodes: []Node{{ID: "a", Title: "A", Prerequisites: []string{"x"}}}}
	duplicate := Graph{Nodes: []Node{{ID: "a", Title: "A"}, {ID: "a", Title: "A"}}}
	for name, g := range map[string]Graph{"cycle": cycle, "unknown": unknown, "duplicate": duplicate, "empty": {}} {
		if err := g.Validate(); !errors.Is(err, ErrInvalidGraph) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestTopicGradeTable(t *testing.T) {
	for _, tc := range []struct {
		c        Criteria
		assisted bool
		want     Grade
	}{
		{Criteria{2, 2, 2, 2}, false, GradeA},
		{Criteria{2, 2, 2, 2}, true, GradeB},
		{Criteria{2, 2, 1, 1}, false, GradeB},
		{Criteria{1, 1, 1, 1}, true, GradeB},
		{Criteria{1, 1, 1, 1}, false, GradeD},
		{Criteria{1, 2, 0, 0}, false, GradeC},
		{Criteria{2, 1, 0, 1}, true, GradeC},
		{Criteria{0, 2, 2, 2}, false, GradeD},
		{Criteria{1, 0, 0, 1}, true, GradeD},
	} {
		if got := TopicGrade(tc.c, tc.assisted); got != tc.want {
			t.Errorf("TopicGrade(%v, %v) = %s, want %s", tc.c, tc.assisted, got, tc.want)
		}
	}
}

func TestCourseGradeThresholds(t *testing.T) {
	for _, tc := range []struct {
		grades  []Grade
		want    Grade
		percent int
	}{
		{[]Grade{GradeA, GradeA, GradeA, GradeB}, GradeA, 93},
		{[]Grade{GradeA, GradeB}, GradeB, 87},
		{[]Grade{GradeB, GradeB}, GradeB, 75},
		{[]Grade{GradeB, GradeC}, GradeC, 62},
		{[]Grade{GradeC, GradeD}, GradeD, 37},
	} {
		grade, percent := CourseGrade(tc.grades)
		if *grade != tc.want || *percent != tc.percent {
			t.Errorf("CourseGrade(%v) = %s %d, want %s %d", tc.grades, *grade, *percent, tc.want, tc.percent)
		}
	}
	if grade, percent := CourseGrade(nil); grade != nil || percent != nil {
		t.Fatal("no graded topics give no grade")
	}
}

func pair() Graph {
	return Graph{Nodes: []Node{
		{ID: "base", Title: "База"},
		{ID: "top", Title: "Верх", Prerequisites: []string{"base"}},
	}}
}

func TestDontKnowOnTheFollowupZeroesK3AndK4(t *testing.T) {
	// A strong main answer keeps C: K1 and K2 on 2, K3 and K4 on 0.
	e, first := NewExam(pair(), 0)
	steps := run(t, e, first, func(s Step) Evaluation {
		if s.Kind == KindFollowup {
			return unknown
		}
		return strong
	})
	if e.Nodes["top"].Status != StatusIndependent || *e.Nodes["top"].Grade != GradeC {
		t.Fatalf("top = %+v", e.Nodes["top"])
	}
	if c := *e.Nodes["top"].Criteria; c != (Criteria{2, 2, 0, 0}) {
		t.Fatalf("top criteria = %+v", c)
	}
	if got := asked(steps, KindQuestion); !slices.Equal(got, []string{"top"}) {
		t.Fatalf("questions = %v (base is credited)", got)
	}
	if e.Nodes["base"].Status != StatusCredited {
		t.Fatalf("base = %s", e.Nodes["base"].Status)
	}

	// A main answer on C that relied on K3 or K4 falls to D and descends.
	e, first = NewExam(pair(), 0)
	steps = run(t, e, first, func(s Step) Evaluation {
		switch {
		case s.NodeID == "top" && s.Kind == KindQuestion:
			return Evaluation{Verdict: VerdictAnswered, Criteria: Criteria{1, 1, 2, 0}}
		case s.NodeID == "top":
			return unknown
		case s.Kind == KindFollowup:
			return strong
		}
		return partial
	})
	if e.Nodes["top"].Status != StatusGap || *e.Nodes["top"].Grade != GradeD {
		t.Fatalf("top = %+v", e.Nodes["top"])
	}
	if got := asked(steps, KindQuestion); !slices.Equal(got, []string{"top", "base"}) {
		t.Fatalf("questions = %v (the gap descends to base, which is not credited)", got)
	}
	if e.Nodes["base"].Status != StatusIndependent {
		t.Fatalf("base = %s", e.Nodes["base"].Status)
	}
}

func TestGapOverAnUnknownPrerequisiteIsNotListed(t *testing.T) {
	e, first := NewExam(pair(), 0)
	run(t, e, first, func(s Step) Evaluation {
		if s.NodeID == "top" {
			return unknown
		}
		return vague
	})
	if e.Nodes["base"].Status != StatusInsufficient {
		t.Fatalf("base = %s", e.Nodes["base"].Status)
	}
	if got := e.Gaps(); len(got) != 0 {
		t.Fatalf("gaps = %v: the mastery of top's prerequisite is unknown", got)
	}
	// The same holds for a prerequisite left unchecked by the question limit.
	e, first = NewExam(pair(), 1)
	run(t, e, first, func(Step) Evaluation { return unknown })
	if e.Nodes["base"].Status != StatusUnchecked || len(e.Gaps()) != 0 {
		t.Fatalf("base = %s gaps = %v", e.Nodes["base"].Status, e.Gaps())
	}
}
