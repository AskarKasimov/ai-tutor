package diagnostic

import "testing"

func TestOutcomeFindingsAcceptFullMainOnBothScales(t *testing.T) {
	answers := []Answer{
		{CompetencyID: "a", Role: "main", Score: 3, Task: TaskSnapshot{OutcomeID: "new-full"}},
		{CompetencyID: "b", Role: "main", Score: 2, Task: TaskSnapshot{OutcomeID: "legacy-full"}},
		{CompetencyID: "c", Role: "main", Score: 1, Task: TaskSnapshot{OutcomeID: "partial"}},
		{CompetencyID: "c", Role: "basic", Score: 1, Task: TaskSnapshot{OutcomeID: "basic"}},
	}
	got := OutcomeFindings(answers)
	if len(got) != 4 || !got[0].Mastered || !got[1].Mastered || got[2].Mastered || !got[3].Mastered {
		t.Fatalf("findings: %+v", got)
	}
	if !HasLearningGaps(answers) || HasLearningGaps(answers[:2]) {
		t.Fatal("only the partial main is a gap")
	}
}
