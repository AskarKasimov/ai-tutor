package training

import "testing"

func blockSession(blocks int) Session {
	s := Session{Round: 1}
	for i := 0; i < blocks; i++ {
		s.Targets = append(s.Targets, Target{Block: &Block{Main: []Task{{Question: "main"}}, Basics: [][]Task{{{Question: "b1"}}, {{Question: "b2"}}}}})
	}
	s.Current = Exercise{TargetIndex: 0, Stage: StageMain}
	return s
}

func move(s *Session, score int) Step {
	step := s.Advance(score)
	s.Current = Exercise{TargetIndex: step.TargetIndex, Stage: step.Stage, BasicIndex: step.BasicIndex}
	return step
}

func TestSolvedMainMovesToNextBlock(t *testing.T) {
	s := blockSession(2)
	if step := move(&s, 2); step != (Step{TargetIndex: 1, Stage: StageMain}) || !s.Targets[0].Block.Mastered {
		t.Fatalf("solved main: %+v %+v", step, s.Targets[0].Block)
	}
	if step := move(&s, 2); step != (Step{TargetIndex: 0, Stage: StageMain}) || s.Round != 2 {
		t.Fatalf("last block must start a new round: %+v round=%d", step, s.Round)
	}
}

func TestFailedMainGoesThroughTheoryAndBasicsBackToMain(t *testing.T) {
	s := blockSession(2)
	for _, want := range []struct {
		score int
		step  Step
	}{
		{1, Step{TargetIndex: 0, Stage: StageTheory}},
		{0, Step{TargetIndex: 0, Stage: StageBasic, BasicIndex: 0}},
		{2, Step{TargetIndex: 0, Stage: StageBasic, BasicIndex: 1}},
		{2, Step{TargetIndex: 0, Stage: StageMain}},
		{2, Step{TargetIndex: 1, Stage: StageMain}},
	} {
		if step := move(&s, want.score); step != want.step {
			t.Fatalf("got %+v, want %+v", step, want.step)
		}
	}
	if s.Targets[0].Block.Cycles != 1 || s.Round != 1 {
		t.Fatalf("cycles=%d round=%d", s.Targets[0].Block.Cycles, s.Round)
	}
}

func TestFailedBasicRepeatsTheoryInTheSameBlock(t *testing.T) {
	s := blockSession(1)
	move(&s, 0) // main → theory
	move(&s, 0) // theory → basic 1
	move(&s, 2) // basic 1 → basic 2
	if step := move(&s, 1); step != (Step{TargetIndex: 0, Stage: StageTheory}) || s.Targets[0].Block.Cycles != 2 {
		t.Fatalf("failed basic must repeat theory: %+v cycles=%d", step, s.Targets[0].Block.Cycles)
	}
	if step := move(&s, 0); step != (Step{TargetIndex: 0, Stage: StageBasic}) || len(s.Targets[0].Block.BasicScores) != 0 {
		t.Fatalf("theory must restart basics: %+v %+v", step, s.Targets[0].Block)
	}
}

func TestLegacyTargetsKeepTheOriginalCycle(t *testing.T) {
	s := Session{Round: 1, Targets: []Target{{}, {}}}
	if step := s.Advance(0); step != (Step{TargetIndex: 1}) {
		t.Fatalf("legacy next: %+v", step)
	}
	s.Current.TargetIndex = 1
	if step := s.Advance(0); step != (Step{TargetIndex: 0}) || s.Round != 2 {
		t.Fatalf("legacy wrap: %+v %d", step, s.Round)
	}
}

func TestPickRotatesPoolAndFallbackTheoryKeepsEveryNote(t *testing.T) {
	pool := []Task{{Question: "a"}, {Question: "b"}}
	if first, _ := Pick(pool, 0); first.Question != "a" {
		t.Fatal(first)
	}
	if third, _ := Pick(pool, 2); third.Question != "a" {
		t.Fatal(third)
	}
	if _, ok := Pick(nil, 0); ok {
		t.Fatal("empty pool")
	}
	text := FallbackTheory([]TheoryNote{{OutcomeName: "TRUE", Content: "один", IncludeInTest: true}, {OutcomeName: "FALSE", Content: "два"}, {OutcomeName: "empty"}})
	if text != "TRUE\nодин\n\nFALSE\nдва" {
		t.Fatalf("fallback %q", text)
	}
}
