package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, q: db.New(pool)} }
func notFound() error {
	return fault.New(fault.NotFound, "TRAINING_SESSION_NOT_FOUND", "Тренировка не найдена.")
}
func conflict(code, message string) error { return fault.New(fault.Conflict, code, message) }
func decode(raw []byte) (training.Session, error) {
	var s training.Session
	err := json.Unmarshal(raw, &s)
	return s, err
}
func (r *Repository) Get(ctx context.Context, owner, id string) (training.Session, error) {
	raw, err := r.q.ReadTrainingSession(ctx, db.ReadTrainingSessionParams{OwnerID: owner, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Session{}, notFound()
	}
	if err != nil {
		return training.Session{}, err
	}
	return decode(raw)
}
func (r *Repository) ByDiagnostic(ctx context.Context, owner, id string) (training.Session, error) {
	raw, err := r.q.FindTrainingByDiagnostic(ctx, db.FindTrainingByDiagnosticParams{OwnerID: owner, DiagnosticID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Session{}, notFound()
	}
	if err != nil {
		return training.Session{}, err
	}
	return decode(raw)
}
func (r *Repository) ByStartKey(ctx context.Context, owner, key, diagnosticID string) (training.Session, bool, error) {
	row, err := r.q.FindTrainingStartKey(ctx, db.FindTrainingStartKeyParams{OwnerID: owner, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Session{}, false, nil
	}
	if err != nil {
		return training.Session{}, false, err
	}
	if row.DiagnosticID != diagnosticID {
		return training.Session{}, false, conflict("IDEMPOTENCY_KEY_REUSED", "Ключ уже использован для другой диагностики.")
	}
	s, err := r.Get(ctx, owner, row.SessionID)
	return s, true, err
}
func (r *Repository) Practice(ctx context.Context, subjectID string) (int64, []training.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	subject, err := q.LockSubjectForVariant(ctx, subjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Предмет не найден.")
	}
	if err != nil {
		return 0, nil, err
	}
	rows, err := q.ReadVariantCandidates(ctx, subject.ActiveRevision)
	if err != nil {
		return 0, nil, err
	}
	tasks := []training.Task{}
	for _, row := range rows {
		if row.TaskID == nil {
			continue
		}
		var t variant.TaskProfile
		if err = json.Unmarshal(row.ProfileJson, &t); err != nil {
			return 0, nil, err
		}
		if strings.TrimSpace(t.Question) == "" || t.VoiceInstruction == nil || strings.TrimSpace(*t.VoiceInstruction) == "" || t.ReferenceAnswer == nil || strings.TrimSpace(*t.ReferenceAnswer) == "" {
			continue
		}
		task := training.Task{SourceTaskID: t.ID, CompetencyID: t.Competency.ID, CompetencyName: t.Competency.Name, ConstituentID: t.Constituent.ID, ConstituentName: t.Constituent.Name, OutcomeID: t.Outcome.ID, OutcomeName: t.Outcome.Name, Question: t.Question, Options: t.Options, VoiceInstruction: *t.VoiceInstruction, ReferenceAnswer: *t.ReferenceAnswer, AudioAssetID: t.AudioAssetID}
		if t.Criteria != nil {
			task.Criteria = *t.Criteria
		}
		if t.Outcome.TaxonomyCode != nil {
			task.TaxonomyCode = *t.Outcome.TaxonomyCode
		}
		if t.Outcome.ALDLevelCode != nil {
			task.ALDLevelCode = *t.Outcome.ALDLevelCode
		}
		if t.Outcome.EducationalContent != nil {
			task.EducationalContent = *t.Outcome.EducationalContent
		}
		if t.Outcome.Importance != nil {
			task.Importance = *t.Outcome.Importance
		}
		tasks = append(tasks, task)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return subject.ActiveRevision, tasks, nil
}
func (r *Repository) Create(ctx context.Context, s training.Session, key string, expectedRevision *int64) (training.Session, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return s, false, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	if err = q.LockTrainingOwner(ctx, s.OwnerID); err != nil {
		return s, false, err
	}
	if row, e := q.FindTrainingStartKey(ctx, db.FindTrainingStartKeyParams{OwnerID: s.OwnerID, RequestKey: key}); e == nil {
		if row.DiagnosticID != s.DiagnosticID {
			return s, false, conflict("IDEMPOTENCY_KEY_REUSED", "Ключ уже использован для другой диагностики.")
		}
		raw, e := q.ReadTrainingSession(ctx, db.ReadTrainingSessionParams{OwnerID: s.OwnerID, ID: row.SessionID})
		if e != nil {
			return s, false, e
		}
		v, e := decode(raw)
		return v, true, e
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return s, false, e
	}
	status, err := q.LockTrainingDiagnostic(ctx, db.LockTrainingDiagnosticParams{OwnerID: s.OwnerID, ID: s.DiagnosticID})
	if errors.Is(err, pgx.ErrNoRows) {
		return s, false, notFound()
	}
	if err != nil {
		return s, false, err
	}
	if status != "completed" {
		return s, false, conflict("DIAGNOSTIC_SESSION_ACTIVE", "Сначала завершите диагностику.")
	}
	reused := false
	if raw, e := q.FindTrainingByDiagnostic(ctx, db.FindTrainingByDiagnosticParams{OwnerID: s.OwnerID, DiagnosticID: s.DiagnosticID}); e == nil {
		s, err = decode(raw)
		if err != nil {
			return s, false, err
		}
		reused = true
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return s, false, e
	}
	if !reused {
		if s.Mode == "free_practice" {
			subject, e := q.LockSubjectForVariant(ctx, s.SubjectID)
			if e != nil {
				return s, false, e
			}
			if expectedRevision == nil || *expectedRevision != s.PlanRevision || subject.ActiveRevision != s.PlanRevision {
				return s, false, conflict("TRAINING_PREVIEW_STALE", "Карта изменилась. Обновите предпросмотр.")
			}
			if len(s.Targets) == 0 {
				return s, false, conflict("NO_PRACTICE_TASKS", "Нет готовых заданий для тренировки.")
			}
		}
		raw, e := json.Marshal(s)
		if e != nil {
			return s, false, e
		}
		if e = q.InsertTrainingSession(ctx, db.InsertTrainingSessionParams{ID: s.ID, OwnerID: s.OwnerID, DiagnosticID: s.DiagnosticID, SubjectID: s.SubjectID, StateData: raw}); e != nil {
			return s, false, e
		}
		for i, t := range s.Targets {
			raw, e := json.Marshal(t)
			if e != nil {
				return s, false, e
			}
			if e = q.InsertTrainingTarget(ctx, db.InsertTrainingTargetParams{SessionID: s.ID, Position: int32(i), TargetData: raw}); e != nil {
				return s, false, e
			}
		}
		if err = insertExercise(ctx, q, s.ID, s.Current); err != nil {
			return s, false, err
		}
	}
	if err = q.InsertTrainingStartKey(ctx, db.InsertTrainingStartKeyParams{OwnerID: s.OwnerID, RequestKey: key, DiagnosticID: s.DiagnosticID, SessionID: s.ID}); err != nil {
		return s, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return s, false, err
	}
	return s, reused, nil
}
func insertExercise(ctx context.Context, q *db.Queries, sessionID string, e training.Exercise) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return q.InsertTrainingExercise(ctx, db.InsertTrainingExerciseParams{ID: e.ID, SessionID: sessionID, Round: e.Round, TargetIndex: int32(e.TargetIndex), ExerciseData: raw, AudioAssetID: e.Task.AudioAssetID})
}
func (r *Repository) Reserve(ctx context.Context, owner, id, key, fingerprint, exerciseID, token string) (*training.Progress, *training.Reservation, training.Session, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, training.Session{}, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LockTrainingSession(ctx, db.LockTrainingSessionParams{OwnerID: owner, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, training.Session{}, notFound()
	}
	if err != nil {
		return nil, nil, training.Session{}, err
	}
	s, err := decode(row.StateData)
	if err != nil {
		return nil, nil, s, err
	}
	if previous, e := q.FindTrainingAnswer(ctx, db.FindTrainingAnswerParams{SessionID: id, RequestKey: key}); e == nil {
		if previous.Fingerprint != fingerprint {
			return nil, nil, s, conflict("IDEMPOTENCY_KEY_REUSED", "Ключ ответа использован с другим аудио.")
		}
		var p training.Progress
		e = json.Unmarshal(previous.ResponseData, &p)
		return &p, nil, s, e
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return nil, nil, s, e
	}
	if s.Current.ID != exerciseID {
		return nil, nil, s, conflict("TRAINING_EXERCISE_NOT_CURRENT", "Укажите текущее упражнение.")
	}
	reserved := training.Reservation{}
	if len(row.ReservationData) > 0 {
		if err = json.Unmarshal(row.ReservationData, &reserved); err != nil {
			return nil, nil, s, err
		}
		if reserved.Token != "" && row.Leased {
			return nil, nil, s, conflict("TRAINING_ANSWER_IN_PROGRESS", "Ответ уже обрабатывается.")
		}
		if reserved.Key != key || reserved.Fingerprint != fingerprint || reserved.ExerciseID != exerciseID {
			return nil, nil, s, conflict("TRAINING_ANSWER_RETRY_MISMATCH", "Для повтора используйте исходный ключ и аудио.")
		}
	}
	reserved.Key = key
	reserved.Fingerprint = fingerprint
	reserved.ExerciseID = exerciseID
	reserved.Token = token
	raw, err := json.Marshal(reserved)
	if err != nil {
		return nil, nil, s, err
	}
	if err = q.SaveTrainingReservation(ctx, db.SaveTrainingReservationParams{OwnerID: owner, ID: id, ReservationData: raw, Leased: true}); err != nil {
		return nil, nil, s, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, s, err
	}
	return nil, &reserved, s, nil
}
func (r *Repository) Accept(ctx context.Context, owner string, reserved training.Reservation, s training.Session, attempt training.Attempt) (training.Progress, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return training.Progress{}, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LockTrainingSession(ctx, db.LockTrainingSessionParams{OwnerID: owner, ID: s.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Progress{}, notFound()
	}
	if err != nil {
		return training.Progress{}, err
	}
	var current training.Reservation
	if len(row.ReservationData) == 0 {
		return training.Progress{}, conflict("TRAINING_ANSWER_STALE", "Обработка устарела.")
	}
	if err = json.Unmarshal(row.ReservationData, &current); err != nil {
		return training.Progress{}, err
	}
	if !row.Leased || current.Token != reserved.Token || current.Key != reserved.Key || current.Fingerprint != reserved.Fingerprint || current.ExerciseID != attempt.ExerciseID {
		return training.Progress{}, conflict("TRAINING_ANSWER_STALE", "Обработка устарела.")
	}
	previous, err := decode(row.StateData)
	if err != nil {
		return training.Progress{}, err
	}
	if previous.Current.ID != attempt.ExerciseID || attempt.Sequence != previous.AnswerCount+1 {
		return training.Progress{}, conflict("TRAINING_ANSWER_STALE", "Упражнение изменилось.")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return training.Progress{}, err
	}
	if err = q.SaveTrainingState(ctx, db.SaveTrainingStateParams{OwnerID: owner, ID: s.ID, StateData: raw}); err != nil {
		return training.Progress{}, err
	}
	if err = insertExercise(ctx, q, s.ID, s.Current); err != nil {
		return training.Progress{}, err
	}
	p := s.Progress()
	p.Answer = &attempt
	answerJSON, err := json.Marshal(attempt)
	if err != nil {
		return p, err
	}
	responseJSON, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if err = q.InsertTrainingAttempt(ctx, db.InsertTrainingAttemptParams{SessionID: s.ID, Sequence: attempt.Sequence, RequestKey: reserved.Key, Fingerprint: reserved.Fingerprint, ExerciseID: attempt.ExerciseID, AttemptData: answerJSON, ResponseData: responseJSON}); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, err
	}
	return p, nil
}
func (r *Repository) Fail(ctx context.Context, owner, id string, reserved training.Reservation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	row, err := q.LockTrainingSession(ctx, db.LockTrainingSessionParams{OwnerID: owner, ID: id})
	if err != nil {
		return err
	}
	if len(row.ReservationData) == 0 {
		return nil
	}
	var current training.Reservation
	if err = json.Unmarshal(row.ReservationData, &current); err != nil {
		return err
	}
	if current.Token != reserved.Token {
		return nil
	}
	var raw []byte
	if reserved.TranscriptionID != "" {
		reserved.Token = ""
		raw, err = json.Marshal(reserved)
		if err != nil {
			return err
		}
	}
	if err = q.SaveTrainingReservation(ctx, db.SaveTrainingReservationParams{OwnerID: owner, ID: id, ReservationData: raw, Leased: false}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *Repository) History(ctx context.Context, owner, id string, before int64, limit int) ([]training.Attempt, error) {
	if _, err := r.Get(ctx, owner, id); err != nil {
		return nil, err
	}
	rows, err := r.q.ReadTrainingHistory(ctx, db.ReadTrainingHistoryParams{SessionID: id, BeforeSequence: before, PageSize: int32(limit)})
	if err != nil {
		return nil, err
	}
	items := []training.Attempt{}
	for _, row := range rows {
		var a training.Attempt
		if err = json.Unmarshal(row.AttemptData, &a); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, nil
}

var _ application.Repository = (*Repository)(nil)
