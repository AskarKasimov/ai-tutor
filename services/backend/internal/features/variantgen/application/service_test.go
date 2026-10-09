package application

import (
	"errors"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type fixedChooser struct {
	calls int
	index int
}

func (c *fixedChooser) Choose(count int) (int, error) {
	c.calls++
	if c.index >= count {
		return 0, errors.New("bad test chooser index")
	}
	return c.index, nil
}

type fixedIDs struct{ n int }

func (g *fixedIDs) New(prefix string) (string, error) {
	g.n++
	return prefix + "-" + string(rune('0'+g.n)), nil
}

func TestBuildUsesPriorityAndStrictlyLowerBasics(t *testing.T) {
	chooser := &fixedChooser{index: 1}
	service := New(nil, chooser, &fixedIDs{}, func() time.Time { return time.Unix(100, 0) })
	candidates := []variant.CandidateOutcome{
		candidate("c1", "Comp", "low-a", "Low A", "understanding", 3, 10, "low-a-task", 1),
		candidate("c1", "Comp", "low-b", "Low B", "understanding", 2, 20, "low-b-task", 2),
		candidate("c1", "Comp", "main", "Main", "application", 5, 30, "main-task-1", 3),
		candidate("c1", "Comp", "main", "Main", "application", 5, 30, "main-task-2", 3),
		candidate("c1", "Comp", "tie", "Tie", "analysis", 4, 40, "tie-task", 4),
	}
	got, err := service.build("owner", "subject:test", "Тест", 7, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got.MapRevision != 7 || got.SubjectID != "subject:test" || got.SubjectNameSnapshot != "Тест" || got.IncludedCompetencyCount != 1 || got.CreatedAt != 100 {
		t.Fatalf("variant metadata: %+v", got)
	}
	selection := got.Competencies[0]
	if selection.Tasks[0].Task.Outcome.ID != "main" {
		t.Fatalf("main outcome = %s", selection.Tasks[0].Task.Outcome.ID)
	}
	if selection.Tasks[0].Task.ID != "main-task-2" {
		t.Fatalf("random chooser did not select pool member: %s", selection.Tasks[0].Task.ID)
	}
	if selection.Tasks[1].Task.Outcome.ID != "low-a" || selection.Tasks[2].Task.Outcome.ID != "low-b" {
		t.Fatalf("basic order: %s, %s", selection.Tasks[1].Task.Outcome.ID, selection.Tasks[2].Task.Outcome.ID)
	}
	if chooser.calls != 1 {
		t.Fatalf("chooser called %d times, want once for multi-task pool", chooser.calls)
	}
	for _, task := range selection.Tasks {
		if task.Task.Outcome.ID == "tie" {
			t.Fatal("higher Bloom tie-break incorrectly selected as a basic")
		}
	}
}

func TestBuildIncludesMainWithAvailableLowerBloomOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		candidates []variant.CandidateOutcome
		want       []string
	}{
		{"only main", []variant.CandidateOutcome{candidate("c1", "Comp", "main", "Main", "application", 5, 1, "t1", 1)}, []string{"main"}},
		{"one basic and analogs", []variant.CandidateOutcome{
			candidate("c1", "Comp", "main", "Main", "analysis", 5, 1, "t1", 1),
			candidate("c1", "Comp", "main", "Main", "analysis", 5, 1, "t1b", 1),
			candidate("c1", "Comp", "low", "Low", "understanding", 2, 2, "t2", 2)}, []string{"main", "low"}},
		{"no lower Bloom", []variant.CandidateOutcome{
			candidate("c1", "Comp", "main", "Main", "application", 5, 1, "t1", 1),
			candidate("c1", "Comp", "same", "Same", "application", 4, 2, "t2", 2),
			candidate("c1", "Comp", "higher", "Higher", "analysis", 1, 3, "t3", 3)}, []string{"main"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := New(nil, &fixedChooser{}, &fixedIDs{}, time.Now)
			got, err := service.build("owner", "subject:test", "Тест", 1, tc.candidates)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Competencies) != 1 || len(got.SkippedCompetencies) != 0 {
				t.Fatalf("unexpected blocks: %+v", got)
			}
			tasks := got.Competencies[0].Tasks
			if len(tasks) != len(tc.want) {
				t.Fatalf("tasks: %+v", tasks)
			}
			for i, want := range tc.want {
				if tasks[i].Task.Outcome.ID != want {
					t.Fatalf("task %d: %+v", i, tasks[i])
				}
			}
		})
	}
}

func TestBuildSkipsOnlyCompetenciesWithoutReadyEnabledOutcomes(t *testing.T) {
	valid := candidate("included", "Included", "ready", "Ready", "knowledge", 1, 1, "t1", 1)
	disabled := candidate("disabled", "Disabled", "off", "Off", "analysis", 5, 2, "t2", 2)
	disabled.Profile.Outcome.IncludeInTest = new(bool)
	incomplete := candidate("incomplete", "Incomplete", "missing", "Missing", "analysis", 5, 3, "t3", 3)
	incomplete.Profile.ReferenceAnswer = nil
	service := New(nil, &fixedChooser{}, &fixedIDs{}, time.Now)
	got, err := service.build("owner", "subject:test", "Тест", 1, []variant.CandidateOutcome{valid, disabled, incomplete})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Competencies) != 1 || len(got.SkippedCompetencies) != 2 {
		t.Fatalf("unexpected variant: %+v", got)
	}
	for _, skipped := range got.SkippedCompetencies {
		if skipped.Code != "NO_READY_OUTCOMES" || skipped.EligibleOutcomes != 0 {
			t.Fatalf("unexpected skip: %+v", skipped)
		}
	}
	_, err = service.build("owner", "subject:test", "Тест", 1, []variant.CandidateOutcome{disabled, incomplete})
	failure, ok := err.(*fault.Error)
	if !ok || failure.Code != "NO_ELIGIBLE_COMPETENCIES" {
		t.Fatalf("unexpected failure: %v", err)
	}
}

func candidate(compID, compName, outcomeID, outcomeName, taxonomy string, importance int16, order int64, taskID string, taskOrder int32) variant.CandidateOutcome {
	include := true
	voice, answer := "Скажите ответ.", "Эталон."
	importanceValue := importance
	return variant.CandidateOutcome{CompetencySourceOrder: 1, SourceOrder: order, HasTask: true, Profile: variant.TaskProfile{
		ID: taskID, Question: "Вопрос?", Options: []string{}, VoiceInstruction: &voice, ReferenceAnswer: &answer, Origin: "authored", SourceRowIndex: &taskOrder,
		Competency: variant.CompetencyProfile{ID: compID, Name: compName},
		Outcome:    variant.OutcomeProfile{ID: outcomeID, Name: outcomeName, IncludeInTest: &include, TaxonomyCode: &taxonomy, Importance: &importanceValue},
	}}
}
