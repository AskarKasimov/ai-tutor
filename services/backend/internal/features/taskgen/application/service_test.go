package application

import (
	"context"
	"errors"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type sourceStub struct {
	source    Source
	sourceErr error
	outcomes  []Outcome
	listErr   error
	gotID     string
}

func (s *sourceStub) SourceByOutcome(_ context.Context, id string) (Source, error) {
	s.gotID = id
	return s.source, s.sourceErr
}

func (s *sourceStub) ListOutcomes(_ context.Context) ([]Outcome, error) {
	return s.outcomes, s.listErr
}

type generatorStub struct {
	tasks    []Task
	err      error
	gotCount int
	gotSrc   Source
}

func (g *generatorStub) Generate(_ context.Context, source Source, count int) ([]Task, error) {
	g.gotSrc, g.gotCount = source, count
	return g.tasks, g.err
}

func validTask() Task {
	return Task{Question: "Новое задание?", Criteria: "Верный ответ назван.", VoiceInstruction: "Ответьте голосом."}
}

func TestGenerateUsesOutcomeSamplesAndDefaultsCount(t *testing.T) {
	sources := &sourceStub{source: Source{OutcomeID: "o-1", OutcomeName: "Дроби", ConstituentName: "Сост", CompetencyName: "ПК-1", Tasks: []Task{{Question: "Что обозначает дробь?"}}}}
	generator := &generatorStub{tasks: []Task{validTask(), {Question: "Задание 2?", Criteria: "Критерий 2.", VoiceInstruction: "Инструкция 2."}, {Question: "Задание 3?", Criteria: "Критерий 3.", VoiceInstruction: "Инструкция 3."}}}
	got, err := New(sources, generator).Generate(context.Background(), "o-1", 0)
	if err != nil || len(got.Tasks) != 3 || got.OutcomeID != "o-1" || got.OutcomeName != "Дроби" {
		t.Fatalf("unexpected result: %#v, %v", got, err)
	}
	if sources.gotID != "o-1" || generator.gotCount != DefaultCount || generator.gotSrc.Tasks[0].Question != "Что обозначает дробь?" {
		t.Fatalf("samples not forwarded: %#v", generator.gotSrc)
	}
}

func TestGenerateRejectsInvalidInput(t *testing.T) {
	service := New(&sourceStub{}, &generatorStub{})
	var f *fault.Error
	if _, err := service.Generate(context.Background(), "", 1); !errors.As(err, &f) || f.Kind != fault.Invalid {
		t.Fatalf("expected empty outcome id to fail, got %v", err)
	}
	if _, err := service.Generate(context.Background(), "o-1", 99); !errors.As(err, &f) || f.Kind != fault.Invalid {
		t.Fatalf("expected too large count to fail, got %v", err)
	}
}

func TestGenerateRejectsOutcomeWithoutSamples(t *testing.T) {
	service := New(&sourceStub{source: Source{OutcomeID: "o-1"}}, &generatorStub{})
	_, err := service.Generate(context.Background(), "o-1", 1)
	var f *fault.Error
	if !errors.As(err, &f) || f.Kind != fault.NotFound {
		t.Fatalf("expected missing samples to fail, got %v", err)
	}
}

func TestGenerateValidatesModelOutput(t *testing.T) {
	sources := &sourceStub{source: Source{OutcomeID: "o-1", Tasks: []Task{{Question: "Что обозначает дробь?"}}}}
	var f *fault.Error
	// Wrong count.
	if _, err := New(sources, &generatorStub{tasks: []Task{validTask()}}).Generate(context.Background(), "o-1", 2); !errors.As(err, &f) || f.Kind != fault.Upstream {
		t.Fatalf("expected wrong count to fail, got %v", err)
	}
	// Duplicate of an existing sample.
	duplicate := validTask()
	duplicate.Question = "что обозначает   дробь?"
	if _, err := New(sources, &generatorStub{tasks: []Task{duplicate}}).Generate(context.Background(), "o-1", 1); !errors.As(err, &f) || f.Kind != fault.Upstream {
		t.Fatalf("expected duplicate question to fail, got %v", err)
	}
	// Missing criteria.
	empty := validTask()
	empty.Criteria = "  "
	if _, err := New(sources, &generatorStub{tasks: []Task{empty}}).Generate(context.Background(), "o-1", 1); !errors.As(err, &f) || f.Kind != fault.Upstream {
		t.Fatalf("expected empty criteria to fail, got %v", err)
	}
}

func TestOutcomesDelegatesToRepository(t *testing.T) {
	sources := &sourceStub{outcomes: []Outcome{{ID: "o-1", Name: "Дроби", TaskCount: 3}}}
	got, err := New(sources, &generatorStub{}).Outcomes(context.Background())
	if err != nil || len(got) != 1 || got[0].TaskCount != 3 {
		t.Fatalf("unexpected outcomes: %#v, %v", got, err)
	}
}
