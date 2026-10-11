package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
)

type chatRepoFake struct {
	reservation     training.TurnReservation
	result          training.TurnResult
	complete        *training.TurnResult
	check           *training.Check
	commits         int
	failures        int
	transcriptSaves int
	resumeCalls     int
}

func (r *chatRepoFake) Read(context.Context, string, string, string, int64, int) (training.ChatPage, error) {
	return training.ChatPage{}, nil
}
func (r *chatRepoFake) ReserveTurn(_ context.Context, owner, session string, input training.TurnInput) (training.TurnReservation, *training.TurnResult, error) {
	r.reservation = training.TurnReservation{OwnerID: owner, SessionID: session, Exercise: validChatExercise(), History: []training.ChatMessage{{Sequence: 1, Role: training.ChatRoleAssistant, Kind: training.ChatKindWelcome, Text: "С чего начнём?"}}, Input: input, Turn: training.Turn{ID: "turn-1", SessionID: session, ExerciseID: input.ExerciseID, RequestKey: input.Key, ExpectedRevision: input.Revision, Token: "token-1", Status: training.TurnStatusProcessing, Source: input.Source, Text: input.Text, PendingAudio: input.Audio, Operation: input.Kind}}
	return r.reservation, nil, nil
}
func (r *chatRepoFake) ResumeTurn(context.Context, string, string, string, string) (training.TurnReservation, *training.TurnResult, error) {
	r.resumeCalls++
	return r.reservation, r.complete, nil
}
func (r *chatRepoFake) SaveTranscript(_ context.Context, _ training.TurnReservation, id, text string) error {
	r.transcriptSaves++
	r.reservation.Turn.TranscriptionID = &id
	r.reservation.Turn.Text = text
	return nil
}
func (r *chatRepoFake) CommitTurn(_ context.Context, reservation training.TurnReservation, commit training.TurnCommit) (training.TurnResult, error) {
	r.commits++
	r.check = commit.Check
	response := training.ChatTurnResult{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, Revision: reservation.Turn.ExpectedRevision + 1, TurnID: reservation.Turn.ID, Messages: commit.Messages}
	if commit.Check != nil {
		response.LatestCheck = &training.ChatCheckView{Revision: response.Revision, FinalAnswer: commit.Check.FinalAnswer, Source: commit.Check.Source, TranscriptionID: commit.Check.TranscriptionID, Score: commit.Check.Score, MaxScore: commit.Check.MaxScore, Verdict: commit.Check.Verdict}
	}
	return training.TurnResult{Response: response}, nil
}
func (r *chatRepoFake) FailTurn(context.Context, training.TurnReservation, string) error {
	r.failures++
	return nil
}
func (r *chatRepoFake) CancelTurn(context.Context, string, string, string, string) error { return nil }

type chatTutorFake struct {
	context training.TutorContext
	reply   training.TutorReply
	err     error
	calls   int
}

func (t *chatTutorFake) Reply(_ context.Context, value training.TutorContext) (training.TutorReply, error) {
	t.calls++
	t.context = value
	return t.reply, t.err
}

type finalGraderFake struct {
	answer string
	calls  int
	result assessment.Evaluation
	err    error
}

func (g *finalGraderFake) GradeFinal(_ context.Context, _ training.Exercise, answer string) (assessment.Evaluation, error) {
	g.calls++
	g.answer = answer
	return g.result, g.err
}

type chatVoiceFake struct{ calls int }

func (v *chatVoiceFake) Transcribe(_ context.Context, owner string, _ []byte, _ string) (transcription.Transcription, error) {
	v.calls++
	return transcription.Transcription{ID: "transcript-1", OwnerID: owner, Text: "Я предполагаю, что нужен числовой результат."}, nil
}

type chatIDsFake struct{ n int }

func (i *chatIDsFake) New(prefix string) (string, error) {
	i.n++
	return prefix + "-" + string(rune('0'+i.n)), nil
}

func TestSendMessageDoesNotGradeOrAdvanceTraining(t *testing.T) {
	repo := &chatRepoFake{}
	tutor := &chatTutorFake{reply: training.TutorReply{AssistantText: "Что именно нужно получить?", Pedagogy: training.PedagogyQuestion}}
	grader := &finalGraderFake{}
	service := NewChatService(repo, nil, tutor, grader, &chatIDsFake{}, time.Now)
	result, err := service.Send(context.Background(), "owner-1", "session-1", training.TurnInput{ExerciseID: "exercise-1", Key: "key-1", Revision: 0, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: "есть две группы"})
	if err != nil {
		t.Fatal(err)
	}
	if grader.calls != 0 || tutor.calls != 1 || repo.commits != 1 || repo.check != nil || result.Response.Revision != 1 {
		t.Fatalf("ordinary turn changed grading/attempt flow: grade=%d tutor=%d commits=%d result=%+v", grader.calls, tutor.calls, repo.commits, result)
	}
	if tutor.context.StudentText != "есть две группы" || tutor.context.Kind != training.ChatKindMessage {
		t.Fatalf("tutor received wrong student input: %+v", tutor.context)
	}
}

func TestCheckGradesOnlyTheFinalStudentTextAndKeepsFeedbackPrivate(t *testing.T) {
	repo := &chatRepoFake{}
	tutor := &chatTutorFake{reply: training.TutorReply{AssistantText: "Вы связали вывод с нужным признаком.", Pedagogy: training.PedagogyConfirmation}}
	grader := &finalGraderFake{result: assessment.Evaluation{Score: 2, MaxScore: 2, Verdict: "correct", Feedback: []string{"Служебный отзыв.", "Вторая строка.", "Третья строка."}, CriterionResults: []assessment.CriterionResult{{Key: "x", Satisfied: true}}}}
	service := NewChatService(repo, nil, tutor, grader, &chatIDsFake{}, time.Now)
	result, err := service.Send(context.Background(), "owner-1", "session-1", training.TurnInput{ExerciseID: "exercise-1", Key: "key-2", Revision: 0, Kind: training.ChatKindCheck, Source: training.AnswerSourceText, Text: "Регрессия, потому что прогнозируем цену."})
	if err != nil {
		t.Fatal(err)
	}
	if grader.calls != 1 || grader.answer != "Регрессия, потому что прогнозируем цену." || repo.check == nil || repo.check.Source != training.AnswerSourceText {
		t.Fatalf("final answer was not checked as submitted: grader=%+v check=%+v", grader, repo.check)
	}
	if tutor.context.Check == nil || tutor.context.Check.Score != 2 || tutor.context.Check.Feedback != nil {
		t.Fatalf("tutor context leaked grader feedback or missed score: %+v", tutor.context.Check)
	}
	if result.Response.LatestCheck == nil || result.Response.LatestCheck.Score != 2 {
		t.Fatalf("public check missing: %+v", result.Response)
	}
}

func TestRetryAfterTutorFailureReusesSavedVoiceTranscript(t *testing.T) {
	repo := &chatRepoFake{}
	voice := &chatVoiceFake{}
	tutor := &chatTutorFake{err: errors.New("temporary tutor failure")}
	service := NewChatService(repo, voice, tutor, &finalGraderFake{}, &chatIDsFake{}, time.Now)
	input := training.TurnInput{ExerciseID: "exercise-1", Key: "key-3", Revision: 0, Kind: training.ChatKindMessage, Source: training.AnswerSourceVoice, Audio: append(append([]byte("OggS"), make([]byte, 23)...), []byte("OpusHead")...), MediaType: "audio/ogg; codecs=opus"}
	if _, err := service.Send(context.Background(), "owner-1", "session-1", input); err == nil {
		t.Fatal("expected tutor failure")
	}
	if voice.calls != 1 || repo.transcriptSaves != 1 || repo.failures != 1 {
		t.Fatalf("STT result was not persisted before failure: voice=%d saved=%d failed=%d", voice.calls, repo.transcriptSaves, repo.failures)
	}
	tutor.err = nil
	tutor.reply = training.TutorReply{AssistantText: "Какое свойство результата это подтверждает?", Pedagogy: training.PedagogyQuestion}
	result, err := service.Retry(context.Background(), "owner-1", "session-1", "exercise-1", "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if voice.calls != 1 || repo.commits != 1 || result.Response.Messages[0].Text != "Я предполагаю, что нужен числовой результат." {
		t.Fatalf("retry repeated STT or lost original speech: voice=%d commits=%d result=%+v", voice.calls, repo.commits, result)
	}
}

func TestCheckProviderFailureDoesNotCreateAZeroScoreOrCommit(t *testing.T) {
	repo := &chatRepoFake{}
	tutor := &chatTutorFake{reply: training.TutorReply{AssistantText: "Ответ принят.", Pedagogy: training.PedagogyConfirmation}}
	grader := &finalGraderFake{err: errors.New("grader unavailable")}
	service := NewChatService(repo, nil, tutor, grader, &chatIDsFake{}, time.Now)
	_, err := service.Send(context.Background(), "owner-1", "session-1", training.TurnInput{ExerciseID: "exercise-1", Key: "key-fail", Revision: 0, Kind: training.ChatKindCheck, Source: training.AnswerSourceText, Text: "Итоговый ответ ученика"})
	if err == nil || repo.commits != 0 || repo.check != nil || tutor.calls != 0 || repo.failures != 1 {
		t.Fatalf("failed grade became a check or reached tutor: err=%v commits=%d check=%+v tutor=%d failures=%d", err, repo.commits, repo.check, tutor.calls, repo.failures)
	}
}
