package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	trainingapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	trainingmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/modelapi"
	trainingpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/postgres"
	trainingsnapshot "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/snapshot"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

func startTrainingChatFixture(t *testing.T) (*fixture, *trainingpg.Repository, *trainingpg.ChatRepository, string, training.Progress) {
	t.Helper()
	f, _, _, owner, diagnosticSession := trainingFixture(t, 2)
	repository := trainingpg.New(f.pool)
	service := trainingapp.New(repository, f.app.diagnosticStore, nil, nil, trainingsnapshot.Provider{}, security.IDGenerator{}, f.app.now)
	preview, err := service.Preview(t.Context(), owner, diagnosticSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	progress, _, err := service.Start(t.Context(), owner, diagnosticSession.ID, "chat-start", &preview.PlanRevision)
	if err != nil {
		t.Fatal(err)
	}
	chats := trainingpg.NewChatRepository(f.pool)
	if _, err = chats.Read(t.Context(), owner, progress.ID, progress.Current.ID, 0, 40); err != nil {
		t.Fatal(err)
	}
	return f, repository, chats, owner, progress
}

func TestTrainingChatReadCreatesOnePersistentWelcome(t *testing.T) {
	f, _, chats, owner, progress := startTrainingChatFixture(t)
	first, err := chats.Read(t.Context(), owner, progress.ID, progress.Current.ID, 0, 40)
	if err != nil {
		t.Fatal(err)
	}
	second, err := chats.Read(t.Context(), owner, progress.ID, progress.Current.ID, 0, 40)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 0 || first.Status != "active" || len(first.Items) != 1 || first.Items[0].Sequence != 1 || first.Items[0].Kind != "welcome" {
		t.Fatalf("unexpected initial chat: %+v", first)
	}
	if len(second.Items) != 1 || second.Items[0].ID != first.Items[0].ID {
		t.Fatalf("repeated read created another welcome: first=%+v second=%+v", first, second)
	}
	_, _, otherOwner := f.register(t, "foreign-training-chat@example.edu")
	if _, err = chats.Read(t.Context(), otherOwner, progress.ID, progress.Current.ID, 0, 40); err == nil {
		t.Fatal("another owner read a private chat")
	} else {
		var f *fault.Error
		if !errors.As(err, &f) || f.Kind != fault.NotFound {
			t.Fatalf("foreign read must be hidden as not found, got %v", err)
		}
	}
}

func TestTrainingChatAllowsOnlyOneWriterPerSession(t *testing.T) {
	_, _, chats, owner, progress := startTrainingChatFixture(t)
	start := make(chan struct{})
	type outcome struct {
		err error
	}
	results := make(chan outcome, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			input := training.TurnInput{ExerciseID: progress.Current.ID, Key: fmt.Sprintf("chat-turn-%d", i), Revision: 0, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: fmt.Sprintf("мысль %d", i)}
			_, _, err := chats.ReserveTurn(context.Background(), owner, progress.ID, input)
			results <- outcome{err: err}
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)
	winners, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			winners++
			continue
		}
		var f *fault.Error
		if errors.As(result.err, &f) && (f.Code == "CHAT_TURN_IN_PROGRESS" || f.Code == "CHAT_PENDING_TURN_EXISTS") {
			conflicts++
			continue
		}
		t.Fatalf("unexpected reservation result: %v", result.err)
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("want one winner and one conflict, got winners=%d conflicts=%d", winners, conflicts)
	}
}

func TestLegacyAnswerAndResetCannotBypassChatReservation(t *testing.T) {
	_, legacy, chats, owner, progress := startTrainingChatFixture(t)
	input := training.TurnInput{ExerciseID: progress.Current.ID, Key: "chat-pending", Revision: 0, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: "начинаю рассуждение"}
	reservation, _, err := chats.ReserveTurn(t.Context(), owner, progress.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = legacy.Reserve(t.Context(), owner, progress.ID, "legacy-answer", "legacy-fingerprint", progress.Current.ID, "legacy-token"); err == nil {
		t.Fatal("legacy answer started while chat turn held the session")
	} else {
		var f *fault.Error
		if !errors.As(err, &f) || f.Code != "CHAT_TURN_IN_PROGRESS" {
			t.Fatalf("expected chat reservation conflict, got %v", err)
		}
	}
	if err = legacy.ResetReservation(t.Context(), owner, progress.ID); err == nil {
		t.Fatal("legacy reset erased the unresolved chat reservation")
	} else {
		var f *fault.Error
		if !errors.As(err, &f) || f.Code != "CHAT_TURN_IN_PROGRESS" {
			t.Fatalf("expected unresolved chat conflict, got %v", err)
		}
	}
	if err = chats.CancelTurn(t.Context(), owner, progress.ID, progress.Current.ID, reservation.Turn.ID); err != nil {
		t.Fatal(err)
	}
	if err = legacy.ResetReservation(t.Context(), owner, progress.ID); err != nil {
		t.Fatalf("legacy reset should work after explicit chat cancellation: %v", err)
	}
}

func TestTrainingChatCancelFencesLateCommitAndRetry(t *testing.T) {
	_, _, chats, owner, progress := startTrainingChatFixture(t)
	input := training.TurnInput{ExerciseID: progress.Current.ID, Key: "cancelled-chat-turn", Revision: 0, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: "не отправлять результат"}
	reservation, _, err := chats.ReserveTurn(t.Context(), owner, progress.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = chats.CancelTurn(t.Context(), owner, progress.ID, progress.Current.ID, reservation.Turn.ID); err != nil {
		t.Fatal(err)
	}
	_, err = chats.CommitTurn(t.Context(), reservation, training.TurnCommit{Messages: [2]training.ChatMessage{
		{Role: training.ChatRoleUser, Kind: training.ChatKindMessage, Text: input.Text},
		{Role: training.ChatRoleAssistant, Kind: training.ChatKindMessage, Text: "устаревший ответ"},
	}})
	if err == nil {
		t.Fatal("cancelled reservation committed a late response")
	}
	_, _, err = chats.ResumeTurn(t.Context(), owner, progress.ID, progress.Current.ID, reservation.Turn.ID)
	var f *fault.Error
	if !errors.As(err, &f) || f.Code != "CHAT_TURN_CANCELLED" {
		t.Fatalf("retry after cancellation must conflict, got %v", err)
	}
}

func TestTrainingChatMessageStalesPublicCheckButKeepsPrivateHistory(t *testing.T) {
	f, _, chats, owner, progress := startTrainingChatFixture(t)
	checkInput := training.TurnInput{ExerciseID: progress.Current.ID, Key: "checked-turn", Revision: 0, Kind: training.ChatKindCheck, Source: training.AnswerSourceText, Text: "Финальная формулировка ученика"}
	checkReservation, _, err := chats.ReserveTurn(t.Context(), owner, progress.ID, checkInput)
	if err != nil {
		t.Fatal(err)
	}
	privateCheck := &training.Check{FinalAnswer: checkInput.Text, Source: checkInput.Source, Score: 1, MaxScore: 2, Verdict: "partial", CriterionResults: []training.CriterionResult{{Key: "criterion", Satisfied: true, Explanation: "выполнено"}}, Feedback: []string{"служебная строка 1", "служебная строка 2", "служебная строка 3"}}
	if _, err = chats.CommitTurn(t.Context(), checkReservation, training.TurnCommit{Check: privateCheck, PromptVersion: training.TutorPromptVersion, Messages: [2]training.ChatMessage{
		{Role: training.ChatRoleUser, Kind: training.ChatKindCheck, Source: training.AnswerSourceText, Text: checkInput.Text},
		{Role: training.ChatRoleAssistant, Kind: training.ChatKindCheck, Text: "Какой признак стоит уточнить?"},
	}}); err != nil {
		t.Fatal(err)
	}
	page, err := chats.Read(t.Context(), owner, progress.ID, progress.Current.ID, 0, 40)
	if err != nil || page.LatestCheck == nil || page.LatestCheck.Score != 1 {
		t.Fatalf("new check was not current: page=%+v err=%v", page, err)
	}
	messageInput := training.TurnInput{ExerciseID: progress.Current.ID, Key: "after-check", Revision: 1, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: "Я хочу уточнить критерий"}
	messageReservation, _, err := chats.ReserveTurn(t.Context(), owner, progress.ID, messageInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = chats.CommitTurn(t.Context(), messageReservation, training.TurnCommit{PromptVersion: training.TutorPromptVersion, Messages: [2]training.ChatMessage{
		{Role: training.ChatRoleUser, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: messageInput.Text},
		{Role: training.ChatRoleAssistant, Kind: training.ChatKindMessage, Text: "Какую часть вы хотите проверить?"},
	}}); err != nil {
		t.Fatal(err)
	}
	page, err = chats.Read(t.Context(), owner, progress.ID, progress.Current.ID, 0, 40)
	if err != nil || page.LatestCheck != nil {
		t.Fatalf("ordinary message did not stale the public check: page=%+v err=%v", page, err)
	}
	var stored training.Check
	if err = f.pool.QueryRow(t.Context(), `SELECT check_data FROM training_chat_turns WHERE session_id=$1 AND request_key=$2`, progress.ID, checkInput.Key).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.FinalAnswer != checkInput.Text || len(stored.CriterionResults) != 1 || len(stored.Feedback) != 3 {
		t.Fatalf("historical trusted check was not retained: %+v", stored)
	}
}

func TestTrainingChatReplayPrecedesCurrentExerciseAndRejectsChangedBody(t *testing.T) {
	f, repository, chats, owner, progress := startTrainingChatFixture(t)
	input := training.TurnInput{ExerciseID: progress.Current.ID, Key: "stable-chat-key", Revision: 0, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: "моя гипотеза"}
	reservation, replay, err := chats.ReserveTurn(t.Context(), owner, progress.ID, input)
	if err != nil || replay != nil {
		t.Fatalf("reserve: replay=%+v err=%v", replay, err)
	}
	commit := training.TurnCommit{Messages: [2]training.ChatMessage{
		{Role: training.ChatRoleUser, Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: input.Text},
		{Role: training.ChatRoleAssistant, Kind: training.ChatKindMessage, Text: "Какой признак поддерживает эту гипотезу?"},
	}, PromptVersion: "test"}
	committed, err := chats.CommitTurn(t.Context(), reservation, commit)
	if err != nil {
		t.Fatal(err)
	}
	state, err := repository.Get(t.Context(), owner, progress.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.Current.ID = "exercise-after-next"
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(t.Context(), `UPDATE training_sessions SET state_data=$2 WHERE id=$1`, progress.ID, stateJSON); err != nil {
		t.Fatal(err)
	}
	_, replay, err = chats.ReserveTurn(t.Context(), owner, progress.ID, input)
	if err != nil || replay == nil || replay.Response.TurnID != committed.Response.TurnID {
		t.Fatalf("expected the committed response before current-exercise validation, replay=%+v err=%v", replay, err)
	}
	input.Text = "другое тело"
	if _, _, err = chats.ReserveTurn(t.Context(), owner, progress.ID, input); err == nil {
		t.Fatal("changed request with reused idempotency key was accepted")
	} else {
		var f *fault.Error
		if !errors.As(err, &f) || f.Code != "IDEMPOTENCY_KEY_REUSED" {
			t.Fatalf("expected idempotency conflict, got %v", err)
		}
	}
}

type automaticChatGrader struct{ answer string }

func (g *automaticChatGrader) GradeFinal(_ context.Context, _ training.Exercise, answer string) (assessment.Evaluation, error) {
	g.answer = answer
	return assessment.Evaluation{Score: 2, MaxScore: 2, Verdict: "correct", CriterionResults: []assessment.CriterionResult{{Key: "answer_correctness", Satisfied: true}}, Feedback: []string{"one", "two", "three"}}, nil
}

func TestTrainingChatTutorAutomaticallyChecksAndEnablesNext(t *testing.T) {
	f, repository, chats, owner, progress := startTrainingChatFixture(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var snapshot struct {
			Check *json.RawMessage `json:"trusted_check"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &snapshot); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		ready := strings.Contains(request.Messages[len(request.Messages)-1].Content, "потому")
		reply := map[string]any{"answer_ready": ready, "assistant_text": "Какой признак подтверждает выбор?", "pedagogy": "question"}
		if snapshot.Check != nil {
			reply["assistant_text"] = "Вы связали вывод с условием."
			reply["pedagogy"] = "confirmation"
		}
		encoded, _ := json.Marshal(reply)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(encoded)}, "finish_reason": "stop"}}})
	}))
	defer provider.Close()
	grader := &automaticChatGrader{}
	service := trainingapp.NewChatService(chats, nil, trainingmodel.NewTutor(provider.Client(), provider.URL, "test-tutor", time.Second), grader, security.IDGenerator{}, f.app.now)
	input := training.TurnInput{ExerciseID: progress.Current.ID, Key: "auto-first", Kind: training.ChatKindMessage, Source: training.AnswerSourceText, Text: "Это классификация."}
	first, err := service.Send(t.Context(), owner, progress.ID, input)
	if err != nil || first.Response.LatestCheck != nil || grader.answer != "" {
		t.Fatalf("partial turn was graded: %+v %v answer=%q", first, err, grader.answer)
	}
	nextService := trainingapp.New(repository, f.app.diagnosticStore, nil, nil, trainingsnapshot.Provider{}, security.IDGenerator{}, f.app.now).WithCheckedNext(chats)
	if _, err := nextService.NextChecked(t.Context(), owner, progress.ID, progress.Current.ID, "next-before-ready", first.Response.Revision); err == nil {
		t.Fatal("next succeeded before tutor readiness")
	}
	input.Key, input.Revision, input.Text = "auto-second", first.Response.Revision, "потому что модель выбирает один из классов."
	checked, err := service.Send(t.Context(), owner, progress.ID, input)
	if err != nil || checked.Response.LatestCheck == nil || checked.Response.LatestCheck.Revision != checked.Response.Revision {
		t.Fatalf("ready turn was not checked: %+v %v", checked, err)
	}
	if grader.answer != "Это классификация.\n\nпотому что модель выбирает один из классов." {
		t.Fatalf("grading dialogue: %q", grader.answer)
	}
	replay, err := service.Send(t.Context(), owner, progress.ID, input)
	if err != nil || replay.Response.TurnID != checked.Response.TurnID {
		t.Fatalf("auto-check replay: %+v %v", replay, err)
	}
	page, err := chats.Read(t.Context(), owner, progress.ID, progress.Current.ID, 0, 40)
	if err != nil || page.LatestCheck == nil || page.LatestCheck.Score != 2 {
		t.Fatalf("persisted auto-check: %+v %v", page, err)
	}
	next, err := nextService.NextChecked(t.Context(), owner, progress.ID, progress.Current.ID, "auto-next", page.Revision)
	if err != nil || next.Current.ID == progress.Current.ID || next.AnswerCount != 1 {
		t.Fatalf("next after automatic check: %+v %v", next, err)
	}
}
