package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
)

type skipRepository struct {
	Repository
	state    training.Session
	accepted *training.Progress
}

func (r *skipRepository) Reserve(_ context.Context, _, _, key, fingerprint, exerciseID, token string) (*training.Progress, *training.Reservation, training.Session, error) {
	if r.accepted != nil {
		return r.accepted, nil, r.state, nil
	}
	return nil, &training.Reservation{Key: key, Fingerprint: fingerprint, ExerciseID: exerciseID, Token: token}, r.state, nil
}
func (r *skipRepository) Accept(_ context.Context, _ string, _ training.Reservation, state training.Session, attempt training.Attempt) (training.Progress, error) {
	r.state = state
	p := state.Progress()
	p.Answer = &attempt
	r.accepted = &p
	return p, nil
}
func (r *skipRepository) Fail(context.Context, string, string, training.Reservation) error {
	return nil
}

type skipVoice struct{ called bool }

func (v *skipVoice) Transcribe(context.Context, string, []byte, string) (transcription.Transcription, error) {
	v.called = true
	return transcription.Transcription{}, fmt.Errorf("speech must not run")
}

type skipGrader struct{ called bool }

func (g *skipGrader) EvaluateTraining(context.Context, string, string, training.Exercise) (assessment.Evaluation, error) {
	g.called = true
	return assessment.Evaluation{}, fmt.Errorf("grading must not run")
}

type skipProvider struct{}

func (skipProvider) Generate(_ context.Context, target training.Target, _ []training.Attempt) (training.Task, string, error) {
	return target.Source, "test", nil
}

type skipIDs struct{ next int }

func (g *skipIDs) New(prefix string) (string, error) {
	g.next++
	return fmt.Sprintf("%s-%d", prefix, g.next), nil
}

func TestSkipAdvancesWithoutSpeechOrGrading(t *testing.T) {
	task := training.Task{OutcomeID: "outcome-1", OutcomeName: "Topic", Question: "Question", VoiceInstruction: "Answer"}
	repo := &skipRepository{state: training.Session{ID: "session-1", OwnerID: "owner-1", Round: 1, Targets: []training.Target{{Source: task}}, Current: training.Exercise{ID: "exercise-1", Task: task}}}
	voice, grader := &skipVoice{}, &skipGrader{}
	service := New(repo, nil, voice, grader, skipProvider{}, &skipIDs{}, func() time.Time { return time.Unix(10, 0) })
	progress, err := service.Skip(context.Background(), "owner-1", "session-1", "exercise-1", "skip-1")
	if err != nil {
		t.Fatal(err)
	}
	if voice.called || grader.called || progress.Answer == nil || !progress.Answer.Skipped || progress.Answer.Score != 0 || progress.Answer.Text != "Я не знаю. Пропустить" || progress.Current.ID == "exercise-1" || progress.AnswerCount != 1 || progress.Targets[0].LastScore == nil || *progress.Targets[0].LastScore != 0 {
		t.Fatalf("skip did not advance cleanly: %+v, voice=%v grader=%v", progress, voice.called, grader.called)
	}
}
