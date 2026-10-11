package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type masteryRepository struct {
	Repository
	state    training.Session
	material training.Material
}

func (r *masteryRepository) ByStartKey(context.Context, string, string, string) (training.Session, bool, error) {
	return training.Session{}, false, nil
}
func (r *masteryRepository) ByDiagnostic(context.Context, string, string) (training.Session, error) {
	return training.Session{}, fault.New(fault.NotFound, "TRAINING_SESSION_NOT_FOUND", "not found")
}
func (r *masteryRepository) Material(context.Context, string) (training.Material, error) {
	return r.material, nil
}
func (r *masteryRepository) Create(_ context.Context, s training.Session, _ string, _ *int64) (training.Session, bool, error) {
	r.state = s
	return s, false, nil
}
func (r *masteryRepository) Get(context.Context, string, string) (training.Session, error) {
	return r.state, nil
}
func (r *masteryRepository) Reserve(_ context.Context, _, _, key, fingerprint, exerciseID, token string) (*training.Progress, *training.Reservation, training.Session, error) {
	return nil, &training.Reservation{Key: key, Fingerprint: fingerprint, ExerciseID: exerciseID, Token: token}, r.state, nil
}
func (r *masteryRepository) Accept(_ context.Context, _ string, _ training.Reservation, s training.Session, attempt training.Attempt) (training.Progress, error) {
	r.state = s
	p := s.Progress()
	p.Answer = &attempt
	return p, nil
}
func (r *masteryRepository) Fail(context.Context, string, string, training.Reservation) error {
	return nil
}
func (r *masteryRepository) AdvanceTheory(_ context.Context, _, exerciseID string, s training.Session) (training.Progress, error) {
	if r.state.Current.ID != exerciseID {
		return training.Progress{}, errors.New("stale theory")
	}
	r.state = s
	return s.Progress(), nil
}

type masteryDiagnostics struct{ session diagnostic.Session }

func (d masteryDiagnostics) Get(context.Context, string, string) (diagnostic.Session, error) {
	return d.session, nil
}

type theoryWriterFake struct{ requests []training.TheoryRequest }

func (w *theoryWriterFake) Summarize(_ context.Context, request training.TheoryRequest) (string, error) {
	w.requests = append(w.requests, request)
	return "Выжимка теории", nil
}

func masteryFixture() (*masteryRepository, masteryDiagnostics) {
	snapshot := func(id, role, outcome, question string) diagnostic.TaskSnapshot {
		return diagnostic.TaskSnapshot{ID: id, SourceTaskID: "source-" + id, Role: role, CompetencyID: "competency", CompetencyName: "Регрессия", OutcomeID: outcome, OutcomeName: outcome, Question: question, VoiceInstruction: "Ответьте", ReferenceAnswer: "Эталон", EducationalContent: "ОС " + outcome}
	}
	competency := diagnostic.Competency{ID: "competency", Name: "Регрессия", Tasks: []diagnostic.TaskSnapshot{
		snapshot("main", "main", "Применяет", "Главная из диагностики"),
		snapshot("basic-1", "basic", "Знает", "Базовая 1"),
		snapshot("basic-2", "basic", "Понимает", "Базовая 2"),
	}}
	session := diagnostic.Session{ID: "diagnostic", Status: diagnostic.StatusCompleted, Variant: diagnostic.VariantSnapshot{SubjectID: "subject", SubjectNameSnapshot: "ML", MapRevision: 3, IncludedCompetencyCount: 1, Competencies: []diagnostic.Competency{competency}},
		Answers: []diagnostic.Answer{{CompetencyID: "competency", OutcomeID: "Применяет", Role: "main", Score: 1, Task: competency.Tasks[0]}, {CompetencyID: "competency", OutcomeID: "Знает", Role: "basic", Score: 1, Task: competency.Tasks[1]}}}
	repository := &masteryRepository{material: training.Material{Revision: 3,
		Tasks: []training.Task{{SourceTaskID: "bank-main", CompetencyName: "Регрессия", OutcomeName: "Применяет", Question: "Другая главная", VoiceInstruction: "Ответьте", ReferenceAnswer: "Эталон"}},
		Notes: map[string][]training.TheoryNote{"Регрессия": {
			{OutcomeName: "Знает", Content: "ОС знает", IncludeInTest: true},
			{OutcomeName: "Не в тесте", Content: "ОС вне теста"},
			{OutcomeName: "Применяет", Content: "ОС применяет", IncludeInTest: true},
		}}}}
	return repository, masteryDiagnostics{session}
}

func TestMasteryBlockMainTheoryBasics(t *testing.T) {
	repository, diagnostics := masteryFixture()
	writer := &theoryWriterFake{}
	service := New(repository, diagnostics, nil, nil, skipProvider{}, &skipIDs{}, func() time.Time { return time.Unix(10, 0) }).WithTheory(writer)
	ctx := context.Background()
	progress, _, err := service.Start(ctx, "owner", "diagnostic", "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress.Targets) != 1 || progress.Targets[0].Kind != "partial_competency" || progress.Current.Stage != training.StageMain || progress.Current.Question != "Другая главная" || progress.Current.BasicCount != 2 {
		t.Fatalf("block must start with a fresh main task: %+v", progress)
	}
	progress, err = service.Skip(ctx, "owner", repository.state.ID, progress.Current.ID, "skip-main")
	if err != nil {
		t.Fatal(err)
	}
	if progress.Current.Stage != training.StageTheory || progress.Current.Theory != "Выжимка теории" || len(writer.requests) != 1 || len(writer.requests[0].Notes) != 3 || writer.requests[0].Notes[1].IncludeInTest {
		t.Fatalf("failed main must open theory over every ОС: %+v %+v", progress.Current, writer.requests)
	}
	theoryID := progress.Current.ID
	var failure *fault.Error
	if _, err = service.Skip(ctx, "owner", repository.state.ID, theoryID, "skip-theory"); !errors.As(err, &failure) || failure.Code != "TRAINING_THEORY_STAGE" {
		t.Fatalf("theory cannot be answered: %v", err)
	}
	progress, err = service.CompleteTheory(ctx, "owner", repository.state.ID, theoryID)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Current.Stage != training.StageBasic || progress.Current.BasicIndex != 0 || progress.Current.Question != "Базовая 1" || progress.AnswerCount != 1 {
		t.Fatalf("theory must lead to the first basic task: %+v", progress)
	}
	if _, err = service.CompleteTheory(ctx, "owner", repository.state.ID, theoryID); !errors.As(err, &failure) || failure.Kind != fault.Conflict {
		t.Fatalf("repeated theory completion must conflict: %v", err)
	}
	progress, _ = service.Skip(ctx, "owner", repository.state.ID, progress.Current.ID, "skip-basic-1")
	progress, _ = service.Skip(ctx, "owner", repository.state.ID, progress.Current.ID, "skip-basic-2")
	if progress.Current.Stage != training.StageTheory || progress.Current.Theory != "Выжимка теории" || len(writer.requests) != 1 {
		t.Fatalf("failed basics must repeat cached theory: %+v requests=%d", progress.Current, len(writer.requests))
	}
}
