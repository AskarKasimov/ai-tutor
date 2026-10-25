package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const chatLeaseSeconds = int64(300)

type ChatRepository struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	leaseSeconds int64
}

func NewChatRepository(pool *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{pool: pool, q: db.New(pool), leaseSeconds: chatLeaseSeconds}
}

func (r *ChatRepository) WithLease(lease time.Duration) *ChatRepository {
	if lease > 0 {
		r.leaseSeconds = int64(lease.Seconds())
	}
	return r
}

func (r *ChatRepository) ReserveNext(ctx context.Context, owner, sessionID, exerciseID, key string, revision int64, token string) (*training.Progress, *training.NextReservation, error) {
	fingerprintBytes := sha256.Sum256([]byte(exerciseID + "\x00next\x00" + fmt.Sprint(revision)))
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: owner, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, notFound()
	}
	if err != nil {
		return nil, nil, err
	}
	state, err := decode(row.StateData)
	if err != nil {
		return nil, nil, err
	}
	if previous, e := q.LearningFindTrainingAnswer(ctx, db.LearningFindTrainingAnswerParams{SessionID: sessionID, RequestKey: key}); e == nil {
		if previous.Fingerprint != fingerprint {
			return nil, nil, conflict("IDEMPOTENCY_KEY_REUSED", "Ключ перехода использован с другими данными.")
		}
		var progress training.Progress
		if e = json.Unmarshal(previous.ResponseData, &progress); e != nil {
			return nil, nil, e
		}
		return &progress, nil, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return nil, nil, e
	}
	if state.Current.ID != exerciseID {
		return nil, nil, conflict("TRAINING_EXERCISE_NOT_CURRENT", "Укажите текущее упражнение.")
	}
	if row.Leased {
		return nil, nil, conflict("TRAINING_ANSWER_IN_PROGRESS", "Тренировка уже обрабатывается.")
	}
	if len(row.ReservationData) > 0 {
		var active training.Reservation
		if err = json.Unmarshal(row.ReservationData, &active); err != nil {
			return nil, nil, err
		}
		if active.Operation == "chat" {
			return nil, nil, conflict("CHAT_PENDING_TURN_EXISTS", "Сначала повторите или отмените ожидающую реплику.")
		}
	}
	chat, err := q.LearningLockTrainingChat(ctx, db.LearningLockTrainingChatParams{SessionID: sessionID, ExerciseID: exerciseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, conflict("CHAT_CHECK_REQUIRED", "Сначала проверьте итоговый ответ для текущего упражнения.")
	}
	if err != nil {
		return nil, nil, err
	}
	if chat.Status != "active" {
		return nil, nil, conflict("CHAT_CHECK_REQUIRED", "Сначала проверьте итоговый ответ для текущего упражнения.")
	}
	if chat.Revision != revision {
		return nil, nil, conflict("CHAT_REVISION_CONFLICT", "Диалог изменился. Обновите историю.")
	}
	pending, err := q.LearningFindTrainingChatPendingTurn(ctx, db.LearningFindTrainingChatPendingTurnParams{SessionID: sessionID, ExerciseID: exerciseID})
	if err == nil {
		_ = pending
		return nil, nil, conflict("CHAT_PENDING_TURN_EXISTS", "Сначала повторите или отмените ожидающую реплику.")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	var check training.Check
	if len(chat.LatestCheckData) == 0 || json.Unmarshal(chat.LatestCheckData, &check) != nil || check.Revision != revision {
		return nil, nil, conflict("CHAT_CHECK_REQUIRED", "Сначала проверьте итоговый ответ для текущего упражнения.")
	}
	reservation := training.Reservation{Key: key, Fingerprint: fingerprint, Token: token, Operation: "next", ExerciseID: exerciseID}
	data, err := json.Marshal(reservation)
	if err != nil {
		return nil, nil, err
	}
	if err = q.LearningSaveTrainingReservationWithLease(ctx, db.LearningSaveTrainingReservationWithLeaseParams{OwnerID: owner, ID: sessionID, ReservationData: data, LeaseSeconds: r.leaseSeconds}); err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return nil, &training.NextReservation{OwnerID: owner, SessionID: sessionID, ExerciseID: exerciseID, Token: token, Key: key, Fingerprint: fingerprint, Session: state, Check: check}, nil
}

func (r *ChatRepository) AcceptNext(ctx context.Context, reservation training.NextReservation, state training.Session, attempt training.Attempt) (training.Progress, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return training.Progress{}, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Progress{}, notFound()
	}
	if err != nil {
		return training.Progress{}, err
	}
	var current training.Reservation
	if len(row.ReservationData) == 0 || json.Unmarshal(row.ReservationData, &current) != nil || !row.Leased || current.Operation != "next" || current.Token != reservation.Token || current.Key != reservation.Key || current.Fingerprint != reservation.Fingerprint {
		return training.Progress{}, conflict("TRAINING_ANSWER_STALE", "Переход устарел.")
	}
	previous, err := decode(row.StateData)
	if err != nil {
		return training.Progress{}, err
	}
	if previous.Current.ID != reservation.ExerciseID || attempt.ExerciseID != reservation.ExerciseID || attempt.Sequence != previous.AnswerCount+1 {
		return training.Progress{}, conflict("TRAINING_ANSWER_STALE", "Упражнение изменилось.")
	}
	chat, err := q.LearningLockTrainingChat(ctx, db.LearningLockTrainingChatParams{SessionID: reservation.SessionID, ExerciseID: reservation.ExerciseID})
	if err != nil {
		return training.Progress{}, err
	}
	var currentCheck training.Check
	if chat.Status != "active" || chat.Revision != reservation.Check.Revision || json.Unmarshal(chat.LatestCheckData, &currentCheck) != nil || currentCheck.Revision != reservation.Check.Revision {
		return training.Progress{}, conflict("CHAT_CHECK_REQUIRED", "Проверку изменили. Проверьте ответ ещё раз.")
	}
	stateData, err := json.Marshal(state)
	if err != nil {
		return training.Progress{}, err
	}
	if err = q.LearningSaveTrainingState(ctx, db.LearningSaveTrainingStateParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID, StateData: stateData}); err != nil {
		return training.Progress{}, err
	}
	if err = insertExercise(ctx, q, reservation.SessionID, state.Current); err != nil {
		return training.Progress{}, err
	}
	progress := state.Progress()
	progress.Answer = &attempt
	answerData, err := json.Marshal(attempt)
	if err != nil {
		return training.Progress{}, err
	}
	responseData, err := json.Marshal(progress)
	if err != nil {
		return training.Progress{}, err
	}
	if err = q.LearningInsertTrainingAttempt(ctx, db.LearningInsertTrainingAttemptParams{SessionID: reservation.SessionID, Sequence: attempt.Sequence, RequestKey: reservation.Key, Fingerprint: reservation.Fingerprint, ExerciseID: reservation.ExerciseID, AttemptData: answerData, ResponseData: responseData}); err != nil {
		return training.Progress{}, err
	}
	if err = q.LearningCloseTrainingChat(ctx, db.LearningCloseTrainingChatParams{SessionID: reservation.SessionID, ExerciseID: reservation.ExerciseID}); err != nil {
		return training.Progress{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return progress, err
	}
	return progress, nil
}

func (r *ChatRepository) FailNext(ctx context.Context, reservation training.NextReservation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID})
	if err != nil {
		return err
	}
	var current training.Reservation
	if len(row.ReservationData) == 0 || json.Unmarshal(row.ReservationData, &current) != nil || current.Token != reservation.Token {
		return nil
	}
	if err = q.LearningSaveTrainingReservation(ctx, db.LearningSaveTrainingReservationParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID, ReservationData: nil, Leased: false}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func chatNotFound() error {
	return fault.New(fault.NotFound, "TRAINING_CHAT_NOT_FOUND", "Диалог не найден.")
}

func chatConflict(code, message string) error { return fault.New(fault.Conflict, code, message) }

func checkView(check *training.Check) *training.ChatCheckView {
	if check == nil {
		return nil
	}
	return &training.ChatCheckView{
		Revision: check.Revision, FinalAnswer: check.FinalAnswer, Source: check.Source,
		TranscriptionID: check.TranscriptionID, Score: check.Score, MaxScore: check.MaxScore,
		Verdict: check.Verdict,
	}
}

func chatMessage(row db.LearningReadTrainingChatPageRow) training.ChatMessage {
	message := training.ChatMessage{ID: row.ID, Sequence: row.Sequence, Role: training.ChatRole(row.Role), Kind: training.ChatKind(row.Kind), Text: row.Text, CreatedAt: row.CreatedAt}
	if row.Source != nil {
		message.Source = training.AnswerSource(*row.Source)
	}
	return message
}

func readTutorHistory(ctx context.Context, q *db.Queries, exerciseID string) ([]training.ChatMessage, string, error) {
	recent, err := q.LearningReadTrainingChatRecent(ctx, db.LearningReadTrainingChatRecentParams{ExerciseID: exerciseID, Limit: 24})
	if err != nil {
		return nil, "", err
	}
	history := make([]training.ChatMessage, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		history = append(history, chatMessage(db.LearningReadTrainingChatPageRow(recent[i])))
	}
	older, err := q.LearningReadTrainingChatSummaryMessages(ctx, exerciseID)
	if err != nil {
		return nil, "", err
	}
	lines := make([]string, 0, len(older))
	for i := len(older) - 1; i >= 0; i-- {
		lines = append(lines, fmt.Sprintf("%d: %s", older[i].Sequence, older[i].Text))
	}
	return history, strings.Join(lines, "\n"), nil
}

type chatRequest struct {
	Revision  int64                 `json:"revision"`
	Kind      training.ChatKind     `json:"kind"`
	Source    training.AnswerSource `json:"source"`
	Text      string                `json:"text,omitempty"`
	MediaType string                `json:"media_type,omitempty"`
}

func requestData(input training.TurnInput) ([]byte, error) {
	return json.Marshal(chatRequest{Revision: input.Revision, Kind: input.Kind, Source: input.Source, Text: input.Text, MediaType: input.MediaType})
}

func turnFingerprint(sessionID string, input training.TurnInput) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00", sessionID, input.ExerciseID, input.Key, input.Revision, input.Kind, input.Source, input.MediaType)
	if input.Source == training.AnswerSourceVoice {
		h.Write(input.Audio)
	} else {
		h.Write([]byte(input.Text))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *ChatRepository) Read(ctx context.Context, ownerID, sessionID, exerciseID string, before int64, limit int) (training.ChatPage, error) {
	if limit <= 0 {
		limit = 40
	}
	if limit > 100 {
		limit = 100
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return training.ChatPage{}, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: ownerID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.ChatPage{}, notFound()
	}
	if err != nil {
		return training.ChatPage{}, err
	}
	session, err := decode(row.StateData)
	if err != nil {
		return training.ChatPage{}, err
	}
	exerciseData, err := q.LearningReadTrainingExercise(ctx, db.LearningReadTrainingExerciseParams{OwnerID: ownerID, SessionID: sessionID, ID: exerciseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.ChatPage{}, chatNotFound()
	}
	if err != nil {
		return training.ChatPage{}, err
	}
	var exercise training.Exercise
	if err = json.Unmarshal(exerciseData, &exercise); err != nil {
		return training.ChatPage{}, err
	}
	chat, err := q.LearningReadTrainingChat(ctx, db.LearningReadTrainingChatParams{SessionID: sessionID, ExerciseID: exerciseID})
	if errors.Is(err, pgx.ErrNoRows) {
		if session.Current.ID != exerciseID {
			return training.ChatPage{}, chatNotFound()
		}
		if err = q.LearningInsertTrainingChat(ctx, db.LearningInsertTrainingChatParams{ExerciseID: exerciseID, SessionID: sessionID}); err != nil {
			return training.ChatPage{}, err
		}
		id, idErr := security.ID("chat_welcome")
		if idErr != nil {
			return training.ChatPage{}, idErr
		}
		if err = q.LearningInsertTrainingChatMessage(ctx, db.LearningInsertTrainingChatMessageParams{ID: id, ExerciseID: exerciseID, Sequence: 1, Role: string(training.ChatRoleAssistant), Kind: string(training.ChatKindWelcome), Text: "С чего начнём? Расскажите, что уже понятно в условии и где возникло затруднение.", ToTimestamp: float64(time.Now().Unix())}); err != nil {
			return training.ChatPage{}, err
		}
		chat, err = q.LearningReadTrainingChat(ctx, db.LearningReadTrainingChatParams{SessionID: sessionID, ExerciseID: exerciseID})
	}
	if err != nil {
		return training.ChatPage{}, err
	}
	check, err := decodeLatestCheck(chat.LatestCheckData, chat.Revision)
	if err != nil {
		return training.ChatPage{}, err
	}
	rows, err := q.LearningReadTrainingChatPage(ctx, db.LearningReadTrainingChatPageParams{ExerciseID: exerciseID, Column2: before, Limit: int32(limit + 1)})
	if err != nil {
		return training.ChatPage{}, err
	}
	page := training.ChatPage{SessionID: sessionID, ExerciseID: exerciseID, Revision: chat.Revision, Status: training.ChatStatus(chat.Status), Items: make([]training.ChatMessage, 0, min(limit, len(rows))), LatestCheck: checkView(check)}
	if len(rows) > limit {
		cursor := fmt.Sprint(rows[limit].Sequence)
		page.NextCursor = &cursor
		rows = rows[:limit]
	}
	for i := len(rows) - 1; i >= 0; i-- {
		page.Items = append(page.Items, chatMessage(rows[i]))
	}
	if pending, e := q.LearningFindTrainingChatPendingTurn(ctx, db.LearningFindTrainingChatPendingTurnParams{SessionID: sessionID, ExerciseID: exerciseID}); e == nil {
		input := chatRequest{}
		if json.Unmarshal(pending.RequestData, &input) == nil {
			retryAfter := int64(0)
			if pending.Status == string(training.TurnStatusProcessing) {
				retryAfter = max(int64(0), pgTime(pending.LeaseUntil).Sub(time.Now()).Milliseconds())
			}
			pendingView := &training.PendingTurnView{ID: pending.ID, Kind: training.ChatKind(pending.Operation), Source: input.Source, Status: training.TurnStatus(pending.Status), RetryAfter: retryAfter}
			if pending.TranscriptionText != nil {
				pendingView.UserText = pending.TranscriptionText
			} else if input.Source == training.AnswerSourceText {
				pendingView.UserText = &input.Text
			}
			page.PendingTurn = pendingView
		}
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return training.ChatPage{}, e
	}
	if err = tx.Commit(ctx); err != nil {
		return training.ChatPage{}, err
	}
	return page, nil
}

func decodeLatestCheck(data []byte, revision int64) (*training.Check, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var check training.Check
	if err := json.Unmarshal(data, &check); err != nil {
		return nil, err
	}
	if !check.IsCurrent(revision) {
		return nil, nil
	}
	return &check, nil
}

func (r *ChatRepository) ReserveTurn(ctx context.Context, ownerID, sessionID string, input training.TurnInput) (training.TurnReservation, *training.TurnResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: ownerID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.TurnReservation{}, nil, notFound()
	}
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	session, err := decode(row.StateData)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	fingerprint := turnFingerprint(sessionID, input)
	if input.Key == "" {
		return training.TurnReservation{}, nil, fault.Validation("Idempotency-Key", "Укажите ключ операции.")
	}
	if previous, e := q.LearningFindTrainingChatTurnByKey(ctx, db.LearningFindTrainingChatTurnByKeyParams{SessionID: sessionID, RequestKey: input.Key}); e == nil {
		if previous.Fingerprint != fingerprint || previous.ExerciseID != input.ExerciseID || previous.Operation != string(input.Kind) {
			return training.TurnReservation{}, nil, chatConflict("IDEMPOTENCY_KEY_REUSED", "Ключ уже использован для другой реплики.")
		}
		if previous.Status == string(training.TurnStatusCompleted) {
			var response training.ChatTurnResult
			if err = json.Unmarshal(previous.ResponseData, &response); err != nil {
				return training.TurnReservation{}, nil, err
			}
			return training.TurnReservation{}, &training.TurnResult{Response: response}, nil
		}
		if previous.Status == string(training.TurnStatusProcessing) && previous.LeaseUntil.Valid && previous.LeaseUntil.Time.After(time.Now()) {
			return training.TurnReservation{}, nil, chatConflict("CHAT_TURN_IN_PROGRESS", "Реплика ещё обрабатывается.")
		}
		return training.TurnReservation{}, nil, chatConflict("CHAT_PENDING_TURN_EXISTS", "Продолжите или отмените ожидающую реплику.")
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return training.TurnReservation{}, nil, e
	}
	if session.Current.ID != input.ExerciseID {
		return training.TurnReservation{}, nil, chatConflict("TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	chat, err := q.LearningLockTrainingChat(ctx, db.LearningLockTrainingChatParams{SessionID: sessionID, ExerciseID: input.ExerciseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.TurnReservation{}, nil, chatNotFound()
	}
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if chat.Status != string(training.ChatStatusActive) {
		return training.TurnReservation{}, nil, chatConflict("TRAINING_CHAT_CLOSED", "Диалог закрыт для записи.")
	}
	if chat.Revision != input.Revision {
		return training.TurnReservation{}, nil, chatConflict("CHAT_REVISION_CONFLICT", "Диалог обновился. Загрузите последние сообщения.")
	}
	if _, e := q.LearningFindTrainingChatPendingTurn(ctx, db.LearningFindTrainingChatPendingTurnParams{SessionID: sessionID, ExerciseID: input.ExerciseID}); e == nil {
		return training.TurnReservation{}, nil, chatConflict("CHAT_PENDING_TURN_EXISTS", "Сначала повторите или отмените ожидающую реплику.")
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return training.TurnReservation{}, nil, e
	}
	if len(row.ReservationData) > 0 {
		var existing training.Reservation
		if err = json.Unmarshal(row.ReservationData, &existing); err != nil {
			return training.TurnReservation{}, nil, err
		}
		if row.Leased {
			return training.TurnReservation{}, nil, chatConflict("CHAT_TURN_IN_PROGRESS", "Другая операция тренировки уже выполняется.")
		}
		return training.TurnReservation{}, nil, chatConflict("CHAT_PENDING_TURN_EXISTS", "Сначала повторите или отмените ожидающую операцию.")
	}
	turnID, err := security.ID("chat_turn")
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	token, err := security.Token()
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	reqData, err := requestData(input)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if err = q.LearningInsertTrainingChatTurn(ctx, db.LearningInsertTrainingChatTurnParams{ID: turnID, SessionID: sessionID, ExerciseID: input.ExerciseID, RequestKey: input.Key, Operation: string(input.Kind), Fingerprint: fingerprint, ExpectedRevision: input.Revision, Token: token, Column9: float64(r.leaseSeconds), RequestData: reqData, PendingAudio: input.Audio}); err != nil {
		return training.TurnReservation{}, nil, err
	}
	reservation := training.Reservation{Key: input.Key, Fingerprint: fingerprint, Token: token, Operation: "chat", ExerciseID: input.ExerciseID, TurnID: turnID, Revision: input.Revision}
	reservationData, err := json.Marshal(reservation)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if err = q.LearningSaveTrainingReservationWithLease(ctx, db.LearningSaveTrainingReservationWithLeaseParams{OwnerID: ownerID, ID: sessionID, ReservationData: reservationData, LeaseSeconds: r.leaseSeconds}); err != nil {
		return training.TurnReservation{}, nil, err
	}
	exerciseData, err := q.LearningReadTrainingExercise(ctx, db.LearningReadTrainingExerciseParams{OwnerID: ownerID, SessionID: sessionID, ID: input.ExerciseID})
	if err != nil {
		return training.TurnReservation{}, nil, chatNotFound()
	}
	var exercise training.Exercise
	if err = json.Unmarshal(exerciseData, &exercise); err != nil {
		return training.TurnReservation{}, nil, err
	}
	history, summary, err := readTutorHistory(ctx, q, input.ExerciseID)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return training.TurnReservation{}, nil, err
	}
	turn := training.Turn{ID: turnID, SessionID: sessionID, ExerciseID: input.ExerciseID, RequestKey: input.Key, Operation: input.Kind, Fingerprint: fingerprint, ExpectedRevision: input.Revision, Token: token, LeaseUntil: time.Now().Add(time.Duration(r.leaseSeconds) * time.Second), Status: training.TurnStatusProcessing, Source: input.Source, Text: input.Text, PendingAudio: append([]byte(nil), input.Audio...)}
	return training.TurnReservation{OwnerID: ownerID, SessionID: sessionID, Exercise: exercise, Turn: turn, History: history, Input: input, Summary: summary}, nil, nil
}

func (r *ChatRepository) ResumeTurn(ctx context.Context, ownerID, sessionID, exerciseID, turnID string) (training.TurnReservation, *training.TurnResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	sessionRow, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: ownerID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.TurnReservation{}, nil, notFound()
	}
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	session, err := decode(sessionRow.StateData)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	turn, err := q.LearningLockTrainingChatTurn(ctx, db.LearningLockTrainingChatTurnParams{SessionID: sessionID, ExerciseID: exerciseID, ID: turnID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.TurnReservation{}, nil, chatNotFound()
	}
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if turn.Status == string(training.TurnStatusCompleted) {
		var response training.ChatTurnResult
		if err = json.Unmarshal(turn.ResponseData, &response); err != nil {
			return training.TurnReservation{}, nil, err
		}
		return training.TurnReservation{}, &training.TurnResult{Response: response}, nil
	}
	if turn.Status == string(training.TurnStatusCancelled) {
		return training.TurnReservation{}, nil, chatConflict("CHAT_TURN_CANCELLED", "Эта реплика отменена.")
	}
	if session.Current.ID != exerciseID {
		return training.TurnReservation{}, nil, chatConflict("TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	token, err := security.Token()
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	affected, err := q.LearningRenewTrainingChatTurn(ctx, db.LearningRenewTrainingChatTurnParams{SessionID: sessionID, ExerciseID: exerciseID, ID: turnID, Token: token, Column5: float64(r.leaseSeconds)})
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if affected != 1 {
		return training.TurnReservation{}, nil, chatConflict("CHAT_TURN_IN_PROGRESS", "Реплика ещё обрабатывается.")
	}
	reservationData, err := json.Marshal(training.Reservation{Key: turn.RequestKey, Fingerprint: turn.Fingerprint, Token: token, Operation: "chat", ExerciseID: exerciseID, TurnID: turnID, Revision: turn.ExpectedRevision})
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	if err = q.LearningSaveTrainingReservationWithLease(ctx, db.LearningSaveTrainingReservationWithLeaseParams{OwnerID: ownerID, ID: sessionID, ReservationData: reservationData, LeaseSeconds: r.leaseSeconds}); err != nil {
		return training.TurnReservation{}, nil, err
	}
	if session.Current.ID != exerciseID {
		return training.TurnReservation{}, nil, chatConflict("TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	var input chatRequest
	if err = json.Unmarshal(turn.RequestData, &input); err != nil {
		return training.TurnReservation{}, nil, err
	}
	exerciseData, err := q.LearningReadTrainingExercise(ctx, db.LearningReadTrainingExerciseParams{OwnerID: ownerID, SessionID: sessionID, ID: exerciseID})
	if err != nil {
		return training.TurnReservation{}, nil, chatNotFound()
	}
	var exercise training.Exercise
	if err = json.Unmarshal(exerciseData, &exercise); err != nil {
		return training.TurnReservation{}, nil, err
	}
	history, summary, err := readTutorHistory(ctx, q, exerciseID)
	if err != nil {
		return training.TurnReservation{}, nil, err
	}
	transcriptionID := turn.TranscriptionID
	turnValue := training.Turn{ID: turn.ID, SessionID: sessionID, ExerciseID: exerciseID, RequestKey: turn.RequestKey, Operation: training.ChatKind(turn.Operation), Fingerprint: turn.Fingerprint, ExpectedRevision: turn.ExpectedRevision, Token: token, Status: training.TurnStatusProcessing, Source: input.Source, Text: input.Text, TranscriptionID: transcriptionID, PendingAudio: append([]byte(nil), turn.PendingAudio...)}
	if turn.TranscriptionText != nil {
		turnValue.Text = *turn.TranscriptionText
	}
	reservation := training.TurnReservation{OwnerID: ownerID, SessionID: sessionID, Exercise: exercise, Turn: turnValue, History: history, Summary: summary, Input: training.TurnInput{ExerciseID: exerciseID, Key: turn.RequestKey, Revision: turn.ExpectedRevision, Kind: training.ChatKind(turn.Operation), Source: input.Source, Text: input.Text, Audio: append([]byte(nil), turn.PendingAudio...), MediaType: input.MediaType}}
	if err = tx.Commit(ctx); err != nil {
		return training.TurnReservation{}, nil, err
	}
	return reservation, nil, nil
}

func (r *ChatRepository) SaveTranscript(ctx context.Context, reservation training.TurnReservation, transcriptionID, text string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID})
	if err != nil {
		return err
	}
	var current training.Reservation
	if json.Unmarshal(row.ReservationData, &current) != nil || current.Token != reservation.Turn.Token || current.TurnID != reservation.Turn.ID {
		return chatConflict("CHAT_TURN_STALE", "Реплика больше не обрабатывается.")
	}
	id, value := transcriptionID, text
	affected, err := q.LearningSaveTrainingChatTranscript(ctx, db.LearningSaveTrainingChatTranscriptParams{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, ID: reservation.Turn.ID, TranscriptionID: &id, TranscriptionText: &value, Token: reservation.Turn.Token})
	if err != nil {
		return err
	}
	if affected != 1 {
		return chatConflict("CHAT_TURN_STALE", "Реплика больше не обрабатывается.")
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *ChatRepository) CommitTurn(ctx context.Context, reservation training.TurnReservation, commit training.TurnCommit) (training.TurnResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return training.TurnResult{}, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	sessionRow, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.TurnResult{}, notFound()
	}
	if err != nil {
		return training.TurnResult{}, err
	}
	var current training.Reservation
	if err = json.Unmarshal(sessionRow.ReservationData, &current); err != nil || current.Token != reservation.Turn.Token || current.TurnID != reservation.Turn.ID || current.Operation != "chat" {
		return training.TurnResult{}, chatConflict("CHAT_TURN_STALE", "Реплика больше не обрабатывается.")
	}
	session, err := decode(sessionRow.StateData)
	if err != nil {
		return training.TurnResult{}, err
	}
	chat, err := q.LearningLockTrainingChat(ctx, db.LearningLockTrainingChatParams{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID})
	if err != nil {
		return training.TurnResult{}, err
	}
	if session.Current.ID != reservation.Turn.ExerciseID || chat.Status != string(training.ChatStatusActive) {
		return training.TurnResult{}, chatConflict("TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	if chat.Revision != reservation.Turn.ExpectedRevision {
		return training.TurnResult{}, chatConflict("CHAT_REVISION_CONFLICT", "Диалог обновился.")
	}
	turn, err := q.LearningLockTrainingChatTurn(ctx, db.LearningLockTrainingChatTurnParams{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, ID: reservation.Turn.ID})
	if err != nil {
		return training.TurnResult{}, err
	}
	if turn.Token != reservation.Turn.Token || turn.Status != string(training.TurnStatusProcessing) {
		return training.TurnResult{}, chatConflict("CHAT_TURN_STALE", "Реплика больше не обрабатывается.")
	}
	newRevision := chat.Revision + 1
	checkJSON := []byte(nil)
	var publicCheck *training.ChatCheckView
	if commit.Check != nil {
		commit.Check.Revision = newRevision
		checkJSON, err = json.Marshal(commit.Check)
		if err != nil {
			return training.TurnResult{}, err
		}
		publicCheck = checkView(commit.Check)
	} else if len(chat.LatestCheckData) > 0 {
		var previousCheck training.Check
		if err = json.Unmarshal(chat.LatestCheckData, &previousCheck); err != nil {
			return training.TurnResult{}, err
		}
		publicCheck = checkView(&previousCheck)
	}
	for i := range commit.Messages {
		message := commit.Messages[i]
		if message.Sequence == 0 {
			message.Sequence = chat.Revision*2 + int64(i) + 2
		}
		if message.ID == "" {
			message.ID, err = security.ID("chat_message")
			if err != nil {
				return training.TurnResult{}, err
			}
		}
		var source *string
		if message.Source != "" {
			v := string(message.Source)
			source = &v
		}
		if message.CreatedAt == 0 {
			message.CreatedAt = time.Now().Unix()
		}
		if err = q.LearningInsertTrainingChatMessage(ctx, db.LearningInsertTrainingChatMessageParams{ID: message.ID, ExerciseID: reservation.Turn.ExerciseID, Sequence: message.Sequence, Role: string(message.Role), Kind: string(message.Kind), Source: source, Text: message.Text, ToTimestamp: float64(message.CreatedAt)}); err != nil {
			return training.TurnResult{}, err
		}
		commit.Messages[i] = message
	}
	if err = q.LearningSetTrainingChatLatestCheck(ctx, db.LearningSetTrainingChatLatestCheckParams{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, Revision: newRevision, Column4: checkJSON}); err != nil {
		return training.TurnResult{}, err
	}
	response := training.ChatTurnResult{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, Revision: newRevision, TurnID: reservation.Turn.ID, Messages: commit.Messages, LatestCheck: publicCheck}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return training.TurnResult{}, err
	}
	var checkData []byte
	if commit.Check != nil {
		checkData, err = json.Marshal(commit.Check)
		if err != nil {
			return training.TurnResult{}, err
		}
	}
	affected, err := q.LearningCompleteTrainingChatTurn(ctx, db.LearningCompleteTrainingChatTurnParams{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, ID: reservation.Turn.ID, ResponseData: responseJSON, PromptVersion: &commit.PromptVersion, Token: reservation.Turn.Token, CheckData: checkData})
	if err != nil {
		return training.TurnResult{}, err
	}
	if affected != 1 {
		return training.TurnResult{}, chatConflict("CHAT_TURN_STALE", "Реплика больше не обрабатывается.")
	}
	if err = q.LearningResetTrainingReservation(ctx, db.LearningResetTrainingReservationParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID}); err != nil {
		return training.TurnResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return training.TurnResult{}, err
	}
	page := training.ChatPage{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, Revision: newRevision, Status: training.ChatStatusActive, Items: commit.Messages[:], LatestCheck: publicCheck}
	return training.TurnResult{Page: page, Response: response}, nil
}

func (r *ChatRepository) FailTurn(ctx context.Context, reservation training.TurnReservation, code string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID})
	if err != nil {
		return err
	}
	var current training.Reservation
	if json.Unmarshal(row.ReservationData, &current) == nil && current.Token == reservation.Turn.Token && current.TurnID == reservation.Turn.ID {
		value := code
		affected, e := q.LearningFailTrainingChatTurn(ctx, db.LearningFailTrainingChatTurnParams{SessionID: reservation.SessionID, ExerciseID: reservation.Turn.ExerciseID, ID: reservation.Turn.ID, ErrorCode: &value, Token: reservation.Turn.Token})
		if e != nil {
			return e
		}
		if affected == 1 {
			current.Token = ""
			data, e := json.Marshal(current)
			if e != nil {
				return e
			}
			if e = q.LearningSaveTrainingReservationWithLease(ctx, db.LearningSaveTrainingReservationWithLeaseParams{OwnerID: reservation.OwnerID, ID: reservation.SessionID, ReservationData: data, LeaseSeconds: 0}); e != nil {
				return e
			}
		}
	}
	return tx.Commit(ctx)
}

func (r *ChatRepository) CancelTurn(ctx context.Context, ownerID, sessionID, exerciseID, turnID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LearningLockTrainingSession(ctx, db.LearningLockTrainingSessionParams{OwnerID: ownerID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}
	if err != nil {
		return err
	}
	turn, err := q.LearningLockTrainingChatTurn(ctx, db.LearningLockTrainingChatTurnParams{SessionID: sessionID, ExerciseID: exerciseID, ID: turnID})
	if errors.Is(err, pgx.ErrNoRows) {
		return chatNotFound()
	}
	if err != nil {
		return err
	}
	if turn.Status == string(training.TurnStatusCompleted) {
		return chatConflict("CHAT_TURN_ALREADY_COMPLETED", "Эта реплика уже сохранена.")
	}
	newToken, err := security.Token()
	if err != nil {
		return err
	}
	affected, err := q.LearningCancelTrainingChatTurn(ctx, db.LearningCancelTrainingChatTurnParams{SessionID: sessionID, ExerciseID: exerciseID, ID: turnID, Token: newToken})
	if err != nil {
		return err
	}
	if affected != 1 {
		return chatConflict("CHAT_TURN_ALREADY_COMPLETED", "Эту реплику нельзя отменить.")
	}
	var current training.Reservation
	if json.Unmarshal(row.ReservationData, &current) == nil && current.TurnID == turnID {
		if err = q.LearningResetTrainingReservation(ctx, db.LearningResetTrainingReservationParams{OwnerID: ownerID, ID: sessionID}); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func pgTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

var _ application.ChatRepository = (*ChatRepository)(nil)
