package diagnostic

import "testing"

func TestTrainingTargetsExcludeUntestedAndPerfectAndClassifyFailedBasics(t *testing.T) {
	answers := []Answer{{CompetencyID: "a", Role: "main", Score: 1, Task: TaskSnapshot{OutcomeID: "main-a"}}, {CompetencyID: "a", Role: "basic", Score: 0, Task: TaskSnapshot{OutcomeID: "basic-a"}}, {CompetencyID: "b", Role: "main", Score: 0, Task: TaskSnapshot{OutcomeID: "main-b"}}, {CompetencyID: "c", Role: "main", Score: 2, Task: TaskSnapshot{OutcomeID: "main-c"}}}
	got := TrainingTargets(answers)
	if len(got) != 2 || got[0].Kind != "confirmed_gap" || got[0].Answer.Task.OutcomeID != "basic-a" || got[1].Kind != "partial_competency" || got[1].Answer.Task.OutcomeID != "main-b" {
		t.Fatalf("targets: %+v", got)
	}
}

func TestTrainingTargetsIncludeUserSkippedAnswersAsZero(t *testing.T) {
	answers := []Answer{{CompetencyID: "a", Role: "main", Skipped: true, Task: TaskSnapshot{OutcomeID: "main-a"}}, {CompetencyID: "a", Role: "basic", Skipped: true, Task: TaskSnapshot{OutcomeID: "basic-a"}}}
	if got := TrainingTargets(answers); len(got) != 1 || got[0].Kind != "confirmed_gap" || got[0].Answer.Task.OutcomeID != "basic-a" {
		t.Fatalf("skipped basic should be a zero-score gap: %+v", got)
	}
	if got := TrainingTargets(answers[:1]); len(got) != 1 || got[0].Kind != "partial_competency" || got[0].Answer.Task.OutcomeID != "main-a" {
		t.Fatalf("skipped main should be a zero-score target: %+v", got)
	}
}
