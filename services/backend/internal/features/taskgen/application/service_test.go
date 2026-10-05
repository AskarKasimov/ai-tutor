package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type testRepository struct {
	getCalls, contextCalls, persistCalls int
	getErr                               error
	context                              Context
	stored                               map[string]Task
	persisted                            Task
	persistArgs                          struct {
		requestKey, requestedBy, model string
		draft                          Draft
		createdAt                      int64
	}
}

func (r *testRepository) GetByKey(_ context.Context, outcomeID, key string) (Task, error) {
	r.getCalls++
	if r.getErr != nil {
		return Task{}, r.getErr
	}
	if task, ok := r.stored[outcomeID+"/"+key]; ok {
		return task, nil
	}
	return Task{}, fault.New(fault.NotFound, "TASK_NOT_FOUND", "not found")
}

func (r *testRepository) Context(context.Context, string) (Context, error) {
	r.contextCalls++
	return r.context, nil
}

func (r *testRepository) Persist(_ context.Context, _ Context, requestKey, requestedBy, model string, draft Draft, createdAt int64) (Task, error) {
	r.persistCalls++
	r.persistArgs.requestKey, r.persistArgs.requestedBy, r.persistArgs.model = requestKey, requestedBy, model
	r.persistArgs.draft, r.persistArgs.createdAt = draft, createdAt
	if r.stored == nil {
		r.stored = make(map[string]Task)
	}
	r.stored["outcome-1/"+requestKey] = r.persisted
	return r.persisted, nil
}

type testGenerator struct {
	calls int
	draft Draft
	err   error
}

func (g *testGenerator) Generate(context.Context, Context) (Draft, error) {
	g.calls++
	return g.draft, g.err
}

func TestGeneratePersistsOnceAndReusesTaskForIdempotencyKey(t *testing.T) {
	repository := &testRepository{
		context:   Context{Revision: 8, Outcome: Outcome{ID: "outcome-1", Name: "ОР"}},
		persisted: Task{ID: "task-1", OutcomeID: "outcome-1", Question: "Сгенерированный вопрос", Origin: "ai_generated"},
	}
	generator := &testGenerator{draft: Draft{Question: "Вопрос", VoiceInstruction: "Объясните", ReferenceAnswer: "Ответ"}}
	service := New(repository, generator, "test-model", func() int64 { return 42 })

	first, err := service.Generate(context.Background(), "outcome-1", "request-1", "student-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Generate(context.Background(), "outcome-1", "request-1", "student-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.ID != "task-1" {
		t.Fatalf("idempotent result differs: first=%#v second=%#v", first, second)
	}
	if repository.getCalls != 2 || repository.contextCalls != 1 || repository.persistCalls != 1 || generator.calls != 1 {
		t.Fatalf("calls: reads=%d contexts=%d persists=%d generations=%d", repository.getCalls, repository.contextCalls, repository.persistCalls, generator.calls)
	}
	if repository.persistArgs.requestKey != "request-1" || repository.persistArgs.requestedBy != "student-1" || repository.persistArgs.model != "test-model" || repository.persistArgs.createdAt != 42 {
		t.Fatalf("persist metadata: %#v", repository.persistArgs)
	}
	if repository.persistArgs.draft.Options == nil {
		t.Fatal("nil options should be normalized to an empty list before persistence")
	}
}

func TestGenerateRejectsInvalidInputBeforeRepositoryAccess(t *testing.T) {
	repository := &testRepository{}
	generator := &testGenerator{}
	service := New(repository, generator, "model", func() int64 { return 0 })
	for _, tc := range []struct{ outcome, key, user string }{
		{"", "key", "user"}, {"outcome", "\x00", "user"}, {"outcome", "key", "  "},
	} {
		if _, err := service.Generate(context.Background(), tc.outcome, tc.key, tc.user); err == nil {
			t.Errorf("invalid input was accepted: %#v", tc)
		}
	}
	if repository.getCalls != 0 || generator.calls != 0 {
		t.Fatalf("invalid inputs reached dependencies: reads=%d generations=%d", repository.getCalls, generator.calls)
	}
}

func TestGenerateStopsOnUnexpectedIdempotencyLookupError(t *testing.T) {
	lookupErr := errors.New("database unavailable")
	repository := &testRepository{getErr: lookupErr}
	generator := &testGenerator{}
	service := New(repository, generator, "model", func() int64 { return 0 })
	if _, err := service.Generate(context.Background(), "outcome-1", "key", "user"); !errors.Is(err, lookupErr) {
		t.Fatalf("lookup error = %v, want wrapped database error", err)
	}
	if repository.contextCalls != 0 || generator.calls != 0 || repository.persistCalls != 0 {
		t.Fatalf("unexpected work after lookup failure: %#v", repository)
	}
}

func TestGenerateDoesNotPersistInvalidModelDraft(t *testing.T) {
	repository := &testRepository{context: Context{Outcome: Outcome{ID: "outcome-1"}}}
	generator := &testGenerator{draft: Draft{Question: "Вопрос", VoiceInstruction: "Инструкция", ReferenceAnswer: "Ответ", Options: []string{" "}}}
	service := New(repository, generator, "model", func() int64 { return 1 })
	if _, err := service.Generate(context.Background(), "outcome-1", "key", "user"); err == nil {
		t.Fatal("draft with a blank answer option was accepted")
	}
	if repository.persistCalls != 0 {
		t.Fatalf("invalid draft was persisted %d times", repository.persistCalls)
	}
}
