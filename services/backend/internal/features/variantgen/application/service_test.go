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

func TestLearningBloomNormalizationAndTiesMatchLegacy(t *testing.T) {
	chooser := &fixedChooser{index: 1}
	service := New(nil, chooser, &fixedIDs{}, func() time.Time { return time.Unix(100, 0) })
	candidates := []variant.CandidateOutcome{
		candidate("c1", "Comp", "low-a", "Low A", "understanding", 3, 10, "low-a-task", 1),
		candidate("c1", "Comp", "low-b", "Low B", "understanding", 2, 20, "low-b-task", 2),
		// A higher importance does not outweigh a lower Bloom rank.
		candidate("c1", "Comp", "important", "Important", "application", 5, 30, "important-task", 3),
		candidate("c1", "Comp", "main", "Main", "analysis", 2, 40, "main-task-1", 4),
		candidate("c1", "Comp", "main", "Main", "analysis", 2, 40, "main-task-2", 4),
		candidate("c1", "Comp", "rival", "Rival", "analysis", 1, 50, "rival-task", 5),
	}
	got, err := service.build("owner", "subject:test", "Тест", 7, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got.MapRevision != 7 || got.SubjectID != "subject:test" || got.SubjectNameSnapshot != "Тест" || got.IncludedCompetencyCount != 1 || got.CreatedAt != 100 || got.AlgorithmVersion != "competency-map-v4" {
		t.Fatalf("variant metadata: %+v", got)
	}
	learning_tasks := got.Competencies[0].Tasks
	if len(learning_tasks) != 3 || learning_tasks[0].Role != "main" || learning_tasks[1].Role != "basic" || learning_tasks[2].Role != "basic" {
		t.Fatalf("learning_tasks: %+v", learning_tasks)
	}
	if learning_tasks[0].Task.Outcome.ID != "main" {
		t.Fatalf("main outcome = %s, want the analysis outcome with the higher importance", learning_tasks[0].Task.Outcome.ID)
	}
	if learning_tasks[0].Task.ID != "main-task-2" {
		t.Fatalf("random chooser did not select pool member: %s", learning_tasks[0].Task.ID)
	}
	if learning_tasks[1].Task.Outcome.ID != "low-a" || learning_tasks[2].Task.Outcome.ID != "low-b" {
		t.Fatalf("basics: %s, %s; want the simplest learning_outcomes", learning_tasks[1].Task.Outcome.ID, learning_tasks[2].Task.Outcome.ID)
	}
	if chooser.calls != 1 {
		t.Fatalf("chooser called %d times, want once for multi-task pool", chooser.calls)
	}
}

func TestBuildAllowsBasicsWithTheMainBloomRank(t *testing.T) {
	service := New(nil, &fixedChooser{}, &fixedIDs{}, time.Now)
	got, err := service.build("owner", "subject:test", "Тест", 1, []variant.CandidateOutcome{
		candidate("c1", "Comp", "app-3", "App 3", "application", 3, 1, "t1", 1),
		candidate("c1", "Comp", "app-5", "App 5", "application", 5, 2, "t2", 2),
		candidate("c1", "Comp", "app-4", "App 4", "application", 4, 3, "t3", 3),
	})
	if err != nil {
		t.Fatal(err)
	}
	learning_tasks := got.Competencies[0].Tasks
	want := []string{"app-5", "app-4", "app-3"}
	if len(learning_tasks) != 3 {
		t.Fatalf("learning_tasks: %+v", learning_tasks)
	}
	for i, id := range want {
		if learning_tasks[i].Task.Outcome.ID != id {
			t.Fatalf("task %d = %s, want %s", i, learning_tasks[i].Task.Outcome.ID, id)
		}
	}
}

func TestBuildSkipsCompetencyWithFewerThanThreeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		candidates []variant.CandidateOutcome
	}{
		{"only main", []variant.CandidateOutcome{candidate("c1", "Comp", "main", "Main", "application", 5, 1, "t1", 1)}},
		{"one basic and analogs", []variant.CandidateOutcome{
			candidate("c1", "Comp", "main", "Main", "analysis", 5, 1, "t1", 1),
			candidate("c1", "Comp", "main", "Main", "analysis", 5, 1, "t1b", 1),
			candidate("c1", "Comp", "low", "Low", "understanding", 2, 2, "t2", 2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := New(nil, &fixedChooser{}, &fixedIDs{}, time.Now)
			_, err := service.build("owner", "subject:test", "Тест", 1, tc.candidates)
			failure, ok := err.(*fault.Error)
			if !ok || failure.Code != "NO_ELIGIBLE_COMPETENCIES" || len(failure.Details) != 1 || failure.Details[0].Code != "INSUFFICIENT_DISTINCT_OUTCOMES" {
				t.Fatalf("want competency skipped with INSUFFICIENT_DISTINCT_OUTCOMES, got %v", err)
			}
		})
	}
}

func TestBuildSkipsOnlyCompetenciesWithoutReadyEnabledOutcomes(t *testing.T) {
	valid := candidate("included", "Included", "ready", "Ready", "application", 4, 1, "t1", 1)
	validLowA := candidate("included", "Included", "low-a", "Low A", "knowledge", 3, 4, "t4", 4)
	validLowB := candidate("included", "Included", "low-b", "Low B", "knowledge", 2, 5, "t5", 5)
	disabled := candidate("disabled", "Disabled", "off", "Off", "analysis", 5, 2, "t2", 2)
	disabled.Profile.Outcome.IncludeInTest = new(bool)
	incomplete := candidate("incomplete", "Incomplete", "missing", "Missing", "analysis", 5, 3, "t3", 3)
	incomplete.Profile.ReferenceAnswer = nil
	service := New(nil, &fixedChooser{}, &fixedIDs{}, time.Now)
	got, err := service.build("owner", "subject:test", "Тест", 1, []variant.CandidateOutcome{valid, validLowA, validLowB, disabled, incomplete})
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
