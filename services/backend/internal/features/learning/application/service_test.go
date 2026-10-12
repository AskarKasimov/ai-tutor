package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func minutes(v int) *int { return &v }

func TestBuildItemsFitsImportantOutcomesIntoHours(t *testing.T) {
	outcomes := []learning.Outcome{
		{OutcomeID: "a", Importance: 2, TheoryMinutes: minutes(30), PracticeMinutes: minutes(30)},
		{OutcomeID: "b", Importance: 5, TheoryMinutes: minutes(20), PracticeMinutes: minutes(20)},
		{OutcomeID: "c", Importance: 4}, // defaults: 15 + 15
	}
	items, detail := BuildItems(outcomes, 1)
	// One hour: the most important outcomes first (b 40 min, c 30 min does not fit).
	if !items[1].InPlan || items[0].InPlan || items[2].InPlan || detail != learning.DetailBrief {
		t.Fatalf("1 hour: %+v %s", items, detail)
	}
	if items[2].TheoryMinutes != learning.DefaultTheoryMinutes || items[2].PracticeMinutes != learning.DefaultPracticeMinutes {
		t.Fatalf("defaults: %+v", items[2])
	}
	// Three hours (180 min) hold all 130 minutes.
	items, detail = BuildItems(outcomes, 3)
	if !items[0].InPlan || !items[1].InPlan || !items[2].InPlan || detail != learning.DetailStandard {
		t.Fatalf("3 hours: %+v %s", items, detail)
	}
	if _, detail = BuildItems(outcomes, 10); detail != learning.DetailDeep {
		t.Fatalf("10 hours: %s", detail)
	}
	// At least one outcome is planned even when it alone exceeds the hours.
	items, _ = BuildItems([]learning.Outcome{{OutcomeID: "big", TheoryMinutes: minutes(300), PracticeMinutes: minutes(300)}}, 1)
	if !items[0].InPlan {
		t.Fatal("the only outcome must be planned")
	}
}

type memoryRepo struct {
	plan     learning.Plan
	messages []learning.Message
	outcomes []learning.Outcome
}

func (r *memoryRepo) Outcomes(context.Context, []string) ([]learning.Outcome, error) {
	return r.outcomes, nil
}
func (r *memoryRepo) ByDiagnostic(context.Context, string, string) (learning.Plan, error) {
	if r.plan.ID == "" {
		return learning.Plan{}, fault.New(fault.NotFound, "LEARNING_PLAN_NOT_FOUND", "")
	}
	return r.plan, nil
}
func (r *memoryRepo) Get(ctx context.Context, owner, id string) (learning.Plan, error) {
	return r.ByDiagnostic(ctx, owner, id)
}
func (r *memoryRepo) Create(_ context.Context, plan learning.Plan) (learning.Plan, error) {
	r.plan = plan
	return plan, nil
}
func (r *memoryRepo) Messages(_ context.Context, _ string, item int) ([]learning.Message, error) {
	result := []learning.Message{}
	for _, m := range r.messages {
		if m.ItemIndex == item {
			result = append(result, m)
		}
	}
	return result, nil
}
func (r *memoryRepo) Update(_ context.Context, _, _ string, change func(*learning.Plan) ([]learning.Message, error)) (learning.Plan, error) {
	plan := r.plan
	plan.Items = append([]learning.Item(nil), r.plan.Items...)
	messages, err := change(&plan)
	if err != nil {
		return learning.Plan{}, err
	}
	r.plan = plan
	r.messages = append(r.messages, messages...)
	return plan, nil
}

type diagnostics struct{ session diagnostic.Session }

func (d diagnostics) Get(context.Context, string, string) (diagnostic.Session, error) {
	return d.session, nil
}

type voice struct{ text string }

func (v *voice) Transcribe(context.Context, string, []byte, string) (transcription.Transcription, error) {
	return transcription.Transcription{ID: "t", Text: v.text}, nil
}

// teacher marks answers containing "верно" correct; fail makes Check fail once.
type teacher struct{ fail bool }

func (teacher) Lesson(_ context.Context, input learning.LessonInput) (learning.Lesson, error) {
	return learning.Lesson{Theory: "Теория " + input.Item.OutcomeID, Question: "Вопрос 1"}, nil
}
func (t *teacher) Check(_ context.Context, input learning.CheckInput) (learning.Check, error) {
	if t.fail {
		t.fail = false
		return learning.Check{}, errors.New("model down")
	}
	if input.Answer == "верно" {
		return learning.Check{Correct: true, Feedback: "Отлично"}, nil
	}
	check := learning.Check{Feedback: "Разбор"}
	if !input.LastAttempt {
		check.Question = "Новый вопрос"
	}
	return check, nil
}

type ids struct{}

func (ids) New(prefix string) (string, error) { return prefix + "_1", nil }

func newService(t *testing.T, main int) (*Service, *memoryRepo, *voice, *teacher) {
	t.Helper()
	session := diagnostic.Session{ID: "d", Status: diagnostic.StatusCompleted,
		Variant: diagnostic.VariantSnapshot{SubjectID: "s", SubjectNameSnapshot: "ML", Competencies: []diagnostic.Competency{{ID: "c1", Name: "К1"}, {ID: "c2", Name: "К2"}}},
		Answers: []diagnostic.Answer{{CompetencyID: "c1", Role: "main", Score: main}, {CompetencyID: "c2", Role: "main", Score: 2}},
	}
	repo := &memoryRepo{outcomes: []learning.Outcome{
		{CompetencyID: "c1", CompetencyName: "К1", OutcomeID: "o1", Importance: 3},
		{CompetencyID: "c1", CompetencyName: "К1", OutcomeID: "o2", Importance: 5},
	}}
	v, tc := &voice{}, &teacher{}
	return New(repo, diagnostics{session}, v, tc, ids{}, func() time.Time { return time.Unix(100, 0) }), repo, v, tc
}

func TestLearningPlanTeachesEachOutcomeUntilMastered(t *testing.T) {
	s, repo, v, tc := newService(t, 1)
	ctx := t.Context()
	if _, err := s.Create(ctx, "u", "d", 0); err == nil {
		t.Fatal("zero hours must be rejected")
	}
	plan, err := s.Create(ctx, "u", "d", 5)
	if err != nil {
		t.Fatal(err)
	}
	// Only the competency with a mistake; its outcomes in map order.
	if len(plan.Items) != 2 || plan.Current != 0 || plan.Items[0].Status != learning.ItemCurrent || plan.Items[1].Status != learning.ItemPending {
		t.Fatalf("plan: %+v", plan)
	}
	if again, _ := s.Create(ctx, "u", "d", 9); again.Hours != 5 {
		t.Fatal("a repeated create must return the same plan")
	}
	if _, err = s.Answer(ctx, "u", "plan", []byte("a"), "audio/wav"); !hasCode(err, "LEARNING_LESSON_NOT_OPEN") {
		t.Fatalf("answer before the lesson: %v", err)
	}
	if plan, err = s.Open(ctx, "u", "plan"); err != nil || plan.Items[0].Question != "Вопрос 1" {
		t.Fatalf("open: %+v %v", plan, err)
	}
	if _, err = s.Open(ctx, "u", "plan"); err != nil || len(repo.messages) != 2 {
		t.Fatalf("a lesson is written once: %d %v", len(repo.messages), err)
	}
	// Three wrong answers: explanation and a new question twice, then review.
	v.text = "не то"
	for attempt := 1; attempt <= 2; attempt++ {
		if plan, err = s.Answer(ctx, "u", "plan", []byte("a"), "audio/wav"); err != nil {
			t.Fatal(err)
		}
		if plan.Current != 0 || plan.Items[0].Attempts != attempt || plan.Items[0].Question != "Новый вопрос" {
			t.Fatalf("attempt %d: %+v", attempt, plan)
		}
	}
	if plan, err = s.Answer(ctx, "u", "plan", []byte("a"), "audio/wav"); err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != learning.ItemReview || plan.Current != 1 || plan.Items[1].Status != learning.ItemCurrent {
		t.Fatalf("after the last attempt: %+v", plan)
	}
	// A failed check keeps the answer and can be retried.
	if _, err = s.Open(ctx, "u", "plan"); err != nil {
		t.Fatal(err)
	}
	v.text, tc.fail = "верно", true
	if _, err = s.Answer(ctx, "u", "plan", []byte("a"), "audio/wav"); err == nil {
		t.Fatal("the check failure must surface")
	}
	if repo.plan.PendingAnswer == nil || *repo.plan.PendingAnswer != "верно" {
		t.Fatalf("pending answer: %+v", repo.plan.PendingAnswer)
	}
	if _, err = s.Answer(ctx, "u", "plan", []byte("a"), "audio/wav"); !hasCode(err, "LEARNING_ANSWER_PENDING") {
		t.Fatalf("a second answer while one waits: %v", err)
	}
	if plan, err = s.Retry(ctx, "u", "plan"); err != nil {
		t.Fatal(err)
	}
	if plan.Items[1].Status != learning.ItemPassed || !plan.Completed() || plan.PendingAnswer != nil || plan.Progress() != 100 {
		t.Fatalf("finished plan: %+v", plan)
	}
	kinds := []learning.MessageKind{}
	for _, m := range repo.messages {
		if m.ItemIndex == 1 {
			kinds = append(kinds, m.Kind)
		}
	}
	if len(kinds) != 4 || kinds[0] != learning.KindTheory || kinds[1] != learning.KindPractice || kinds[2] != learning.KindAnswer || kinds[3] != learning.KindFeedback {
		t.Fatalf("second item chat: %v", kinds)
	}
}

func TestLearningPlanWithoutMistakesIsComplete(t *testing.T) {
	s, _, _, _ := newService(t, 2)
	plan, err := s.Create(t.Context(), "u", "d", 3)
	if err != nil || len(plan.Items) != 0 || !plan.Completed() {
		t.Fatalf("%+v %v", plan, err)
	}
}

func hasCode(err error, code string) bool {
	var f *fault.Error
	return errors.As(err, &f) && f.Code == code
}
