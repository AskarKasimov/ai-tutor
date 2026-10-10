package diagnostic

import "testing"

func TestProgressReportsFixedTopicsAndCurrentStep(t *testing.T) {
	session := Session{
		ID: "s", Status: StatusActive, CurrentCompetency: 1, CurrentTask: 2,
		Variant: VariantSnapshot{Competencies: []Competency{
			{Tasks: []TaskSnapshot{{ID: "a"}, {ID: "b"}, {ID: "c"}}},
			{Tasks: []TaskSnapshot{{ID: "d"}, {ID: "e"}, {ID: "f"}}},
		}},
	}
	got := session.Progress()
	if got.CompetencyCount != 2 || got.CurrentCompetency != 2 || got.CurrentStep != 2 || got.Current == nil || got.Current.ID != "f" {
		t.Fatalf("progress = %+v", got)
	}
	session.Status = StatusCompleted
	if done := session.Progress(); done.CompetencyCount != 2 || done.CurrentCompetency != 0 || done.CurrentStep != 0 {
		t.Fatalf("completed progress = %+v", done)
	}
}
