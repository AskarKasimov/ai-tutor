package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type ChatService struct {
	repo   ChatRepository
	voice  Voice
	tutor  Tutor
	grader FinalAnswerGrader
	ids    IDs
	now    func() time.Time
	limits training.ContextLimits
}

func NewChatService(repo ChatRepository, voice Voice, tutor Tutor, grader FinalAnswerGrader, ids IDs, now func() time.Time) *ChatService {
	if ids == nil {
		ids = defaultIDs{}
	}
	if now == nil {
		now = time.Now
	}
	return &ChatService{repo: repo, voice: voice, tutor: tutor, grader: grader, ids: ids, now: now, limits: training.ContextLimits{TaskMaxChars: 16000, HistoryMaxChars: 16000, SummaryMaxChars: 4000}}
}

type defaultIDs struct{}

func (defaultIDs) New(prefix string) (string, error) { return security.ID(prefix) }

func (s *ChatService) WithContextLimits(limits training.ContextLimits) *ChatService {
	s.limits = limits
	return s
}

func (s *ChatService) Read(ctx context.Context, owner, sessionID, exerciseID string, before int64, limit int) (training.ChatPage, error) {
	return s.repo.Read(ctx, owner, sessionID, exerciseID, before, limit)
}

func validateTurnInput(input training.TurnInput) error {
	if err := ValidKey(input.Key); err != nil {
		return err
	}
	if input.Revision < 0 {
		return fault.Validation("revision", "Ревизия должна быть неотрицательным целым числом.")
	}
	if input.Kind != training.ChatKindMessage && input.Kind != training.ChatKindCheck {
		return fault.Validation("kind", "Неизвестный тип реплики.")
	}
	switch input.Source {
	case training.AnswerSourceText:
		if len(input.Audio) != 0 || input.MediaType != "" {
			return fault.Validation("input", "Передайте текст или аудио.")
		}
		if !validChatText(input.Text) {
			return fault.Validation("text", "Текст должен содержать от 1 до 4000 символов.")
		}
	case training.AnswerSourceVoice:
		if input.Text != "" {
			return fault.Validation("input", "Передайте текст или аудио.")
		}
		if err := audio.Check(input.Audio, input.MediaType); err != nil {
			return err
		}
	default:
		return fault.Validation("source", "Укажите текстовый или голосовой ввод.")
	}
	return nil
}

func validChatText(value string) bool {
	text := strings.TrimSpace(value)
	return text != "" && utf8.ValidString(text) && !strings.ContainsRune(text, 0) && utf8.RuneCountInString(text) <= 4000
}

func (s *ChatService) Send(ctx context.Context, owner, sessionID string, input training.TurnInput) (training.TurnResult, error) {
	if err := validateTurnInput(input); err != nil {
		return training.TurnResult{}, err
	}
	reservation, replay, err := s.repo.ReserveTurn(ctx, owner, sessionID, input)
	if err != nil {
		return training.TurnResult{}, err
	}
	if replay != nil {
		return *replay, nil
	}
	return s.runReserved(ctx, reservation)
}

func (s *ChatService) Retry(ctx context.Context, owner, sessionID, exerciseID, turnID string) (training.TurnResult, error) {
	reservation, replay, err := s.repo.ResumeTurn(ctx, owner, sessionID, exerciseID, turnID)
	if err != nil {
		return training.TurnResult{}, err
	}
	if replay != nil {
		return *replay, nil
	}
	return s.runReserved(ctx, reservation)
}

func (s *ChatService) runReserved(ctx context.Context, reservation training.TurnReservation) (result training.TurnResult, returnErr error) {
	defer func() {
		if returnErr == nil {
			return
		}
		code := "CHAT_TURN_FAILED"
		var f *fault.Error
		if errors.As(returnErr, &f) {
			code = f.Code
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.repo.FailTurn(cleanupCtx, reservation, code)
	}()

	studentText := strings.TrimSpace(reservation.Turn.Text)
	var transcriptionID *string
	if reservation.Turn.TranscriptionID != nil {
		id := *reservation.Turn.TranscriptionID
		transcriptionID = &id
	}
	if reservation.Turn.Source == training.AnswerSourceVoice && transcriptionID == nil {
		if s.voice == nil {
			return training.TurnResult{}, fault.New(fault.Unavailable, "VOICE_UNAVAILABLE", "Распознавание речи временно недоступно.")
		}
		transcribed, err := s.voice.Transcribe(ctx, reservation.OwnerID, reservation.Turn.PendingAudio, reservation.Input.MediaType)
		if err != nil {
			return training.TurnResult{}, err
		}
		studentText = strings.TrimSpace(transcribed.Text)
		if transcribed.OwnerID != reservation.OwnerID || strings.TrimSpace(transcribed.ID) == "" || !validChatText(studentText) {
			return training.TurnResult{}, fault.New(fault.Invalid, "NO_SPEECH", "Не удалось распознать голосовую реплику.")
		}
		id := transcribed.ID
		transcriptionID = &id
		if err = s.repo.SaveTranscript(ctx, reservation, id, studentText); err != nil {
			return training.TurnResult{}, err
		}
		reservation.Turn.TranscriptionID = &id
		reservation.Turn.Text = studentText
	}
	if !validChatText(studentText) {
		return training.TurnResult{}, fault.Validation("text", "Текст должен содержать от 1 до 4000 символов.")
	}
	if s.tutor == nil {
		return training.TurnResult{}, fault.New(fault.Unavailable, "TUTOR_UNAVAILABLE", "Репетитор временно недоступен.")
	}
	check := (*training.Check)(nil)
	grade := func(answer string) error {
		if s.grader == nil {
			return fault.New(fault.Unavailable, "ASSESSMENT_UNAVAILABLE", "Проверка ответа временно недоступна.")
		}
		evaluation, err := s.grader.GradeFinal(ctx, reservation.Exercise, answer)
		if err != nil {
			return err
		}
		if err = validateFinalEvaluation(evaluation); err != nil {
			return err
		}
		check = &training.Check{Revision: reservation.Turn.ExpectedRevision + 1, FinalAnswer: answer, Source: reservation.Turn.Source, TranscriptionID: transcriptionID, Score: evaluation.Score, MaxScore: evaluation.MaxScore, Verdict: evaluation.Verdict, CriterionResults: criterionResults(evaluation.CriterionResults), Feedback: append([]string(nil), evaluation.Feedback...)}
		return nil
	}
	// Theory has no reference answer: the tutor only explains it and never grades.
	theory := reservation.Exercise.Stage == training.StageTheory
	if theory && reservation.Turn.Operation == training.ChatKindCheck {
		return training.TurnResult{}, fault.New(fault.Conflict, "TRAINING_THEORY_STAGE", "Сейчас этап теории: изучите её и переходите к задачам.")
	}
	// Keep explicit checks compatible with historical clients. The current UI sends one kind of turn.
	if reservation.Turn.Operation == training.ChatKindCheck {
		if err := grade(studentText); err != nil {
			return training.TurnResult{}, err
		}
	}
	tutorContext, err := BuildTutorContext(reservation.Exercise, reservation.History, training.TutorInput{Text: studentText, Kind: reservation.Turn.Operation, Check: check, Summary: reservation.Summary}, s.limits)
	if err != nil {
		return training.TurnResult{}, err
	}
	reply, err := s.tutor.Reply(ctx, tutorContext)
	if err != nil {
		return training.TurnResult{}, err
	}
	if check == nil && reply.AnswerReady && !theory {
		// Grade only the student's complete recent statements, never tutor suggestions or truncated summaries.
		answers := make([]string, 0, len(tutorContext.Recent)+1)
		for _, message := range tutorContext.Recent {
			if message.Role == training.ChatRoleUser {
				answers = append(answers, message.Text)
			}
		}
		answers = append(answers, studentText)
		if err := grade(strings.Join(answers, "\n\n")); err != nil {
			return training.TurnResult{}, err
		}
		tutorContext.Check = cloneCheck(check)
		reply, err = s.tutor.Reply(ctx, tutorContext)
		if err != nil {
			return training.TurnResult{}, err
		}
	}
	userID, err := s.ids.New("chat_message")
	if err != nil {
		return training.TurnResult{}, err
	}
	assistantID, err := s.ids.New("chat_message")
	if err != nil {
		return training.TurnResult{}, err
	}
	messageKind := training.ChatKindMessage
	if reservation.Turn.Operation == training.ChatKindCheck {
		messageKind = training.ChatKindCheck
	}
	messages := [2]training.ChatMessage{
		{ID: userID, Role: training.ChatRoleUser, Kind: messageKind, Source: reservation.Turn.Source, Text: studentText, CreatedAt: s.now().Unix()},
		{ID: assistantID, Role: training.ChatRoleAssistant, Kind: messageKind, Text: reply.AssistantText, CreatedAt: s.now().Unix()},
	}
	commit := training.TurnCommit{Messages: messages, Check: check, PromptVersion: training.TutorPromptVersion}
	return s.repo.CommitTurn(ctx, reservation, commit)
}

func validateFinalEvaluation(evaluation assessment.Evaluation) error {
	verdicts := map[int]string{0: "incorrect", 1: "partial", 2: "correct"}
	if evaluation.MaxScore != 2 || evaluation.Score < 0 || evaluation.Score > 2 || evaluation.Verdict != verdicts[evaluation.Score] || len(evaluation.CriterionResults) == 0 || len(evaluation.Feedback) != 3 {
		return fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Проверка вернула некорректный результат.")
	}
	return nil
}

func criterionResults(values []assessment.CriterionResult) []training.CriterionResult {
	results := make([]training.CriterionResult, 0, len(values))
	for _, value := range values {
		results = append(results, training.CriterionResult{Key: value.Key, Satisfied: value.Satisfied, Explanation: value.Explanation})
	}
	return results
}

func (s *ChatService) Cancel(ctx context.Context, owner, sessionID, exerciseID, turnID string) error {
	return s.repo.CancelTurn(ctx, owner, sessionID, exerciseID, turnID)
}
