// Package postgres persists diagnostic session start and read state in PostgreSQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, q: db.New(pool)} }

func (s *Store) FindStart(ctx context.Context, ownerID, key, digest string) (diagnostic.Session, bool, error) {
	row, err := s.q.FindDiagnosticSessionByStart(ctx, db.FindDiagnosticSessionByStartParams{OwnerID: ownerID, StartRequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return diagnostic.Session{}, false, nil
	}
	if err != nil {
		return diagnostic.Session{}, false, err
	}
	if row.StartRequestDigest != digest {
		return diagnostic.Session{}, false, startKeyConflict()
	}
	value, err := decodeSession(row.SessionData)
	return value, true, err
}

func (s *Store) Create(ctx context.Context, ownerID, key, digest string, value diagnostic.Session) (diagnostic.Session, bool, error) {
	if value.ID == "" || value.OwnerID != ownerID || value.VariantID == "" || key == "" || digest == "" {
		return diagnostic.Session{}, false, fault.Validation("diagnostic_session", "Данные диагностической сессии некорректны.")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return diagnostic.Session{}, false, err
	}
	rows, err := s.q.CreateDiagnosticSession(ctx, db.CreateDiagnosticSessionParams{
		ID: value.ID, OwnerID: ownerID, VariantID: value.VariantID, Status: value.Status,
		CurrentCompetency: int32(value.CurrentCompetency), CurrentTask: int32(value.CurrentTask),
		StartRequestKey: key, StartRequestDigest: digest, SessionData: encoded,
	})
	if err != nil {
		return diagnostic.Session{}, false, err
	}
	if rows == 0 {
		previous, found, err := s.FindStart(ctx, ownerID, key, digest)
		if err != nil {
			return diagnostic.Session{}, false, err
		}
		if found {
			return previous, true, nil
		}
		return diagnostic.Session{}, false, variantNotFound()
	}
	stored, err := s.Get(ctx, ownerID, value.ID)
	return stored, false, err
}

func (s *Store) Get(ctx context.Context, ownerID, id string) (diagnostic.Session, error) {
	data, err := s.q.GetDiagnosticSession(ctx, db.GetDiagnosticSessionParams{OwnerID: ownerID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return diagnostic.Session{}, sessionNotFound()
	}
	if err != nil {
		return diagnostic.Session{}, err
	}
	return decodeSession(data)
}

func (s *Store) LatestCompleted(ctx context.Context, ownerID, subjectID string) (diagnostic.Session, bool, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT session_data FROM diagnostic_sessions WHERE owner_id=$1 AND subject_id=$2 AND status='completed' ORDER BY completed_at DESC NULLS LAST, id DESC LIMIT 1`, ownerID, subjectID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return diagnostic.Session{}, false, nil
	}
	if err != nil {
		return diagnostic.Session{}, false, err
	}
	value, err := decodeSession(data)
	return value, err == nil, err
}

func (s *Store) Reserve(ctx context.Context, ownerID, id, key, digest, taskID, token string) (*diagnostic.AcceptedRequest, *diagnostic.Reservation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	var data []byte
	if err = tx.QueryRow(ctx, `SELECT session_data FROM diagnostic_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE`, ownerID, id).Scan(&data); errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, sessionNotFound()
	} else if err != nil {
		return nil, nil, err
	}
	value, err := decodeSession(data)
	if err != nil {
		return nil, nil, err
	}
	if previous, ok := value.AcceptedRequests[key]; ok {
		if previous.Fingerprint != digest {
			return nil, nil, startKeyConflict()
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, nil, err
		}
		return &previous, nil, nil
	}
	if value.Status != diagnostic.StatusActive {
		return nil, nil, fault.New(fault.Conflict, "DIAGNOSTIC_SESSION_COMPLETED", "Диагностическая сессия уже завершена.")
	}
	current := value.Current()
	if current == nil || current.ID != taskID {
		return nil, nil, fault.New(fault.Conflict, "DIAGNOSTIC_TASK_NOT_CURRENT", "Укажите текущее задание сессии.")
	}
	if value.InFlight != nil {
		p := value.InFlight
		if p.Token != "" {
			return nil, nil, fault.New(fault.Conflict, "DIAGNOSTIC_ANSWER_IN_PROGRESS", "Ответ для текущего задания уже обрабатывается.")
		}
		if p.Key != key || p.Fingerprint != digest || p.VariantTaskID != taskID {
			return nil, nil, fault.New(fault.Conflict, "DIAGNOSTIC_ANSWER_RETRY_MISMATCH", "Для повтора используйте исходный ключ и аудио.")
		}
		p.Token = token
	} else {
		value.InFlight = &diagnostic.Reservation{Key: key, Fingerprint: digest, VariantTaskID: taskID, Token: token}
	}
	if err = saveLocked(ctx, tx, value); err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	p := *value.InFlight
	return nil, &p, nil
}

func (s *Store) Accept(ctx context.Context, ownerID, id, token, key, digest string, answer diagnostic.Answer, transition diagnostic.Transition) (diagnostic.Progress, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return diagnostic.Progress{}, err
	}
	defer tx.Rollback(ctx)
	var data []byte
	if err = tx.QueryRow(ctx, `SELECT session_data FROM diagnostic_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE`, ownerID, id).Scan(&data); errors.Is(err, pgx.ErrNoRows) {
		return diagnostic.Progress{}, sessionNotFound()
	} else if err != nil {
		return diagnostic.Progress{}, err
	}
	value, err := decodeSession(data)
	if err != nil {
		return diagnostic.Progress{}, err
	}
	if value.InFlight == nil || value.InFlight.Token != token || value.InFlight.VariantTaskID != answer.VariantTaskID {
		return diagnostic.Progress{}, fault.New(fault.Conflict, "DIAGNOSTIC_ANSWER_STALE", "Обработка ответа устарела.")
	}
	value.Answers = append(value.Answers, answer)
	value.CurrentCompetency = transition.CurrentCompetency
	value.CurrentTask = transition.CurrentTask
	value.Status = transition.Status
	value.SkippedBasics = append(value.SkippedBasics, transition.SkippedBasics...)
	value.InFlight = nil
	progress := value.Progress()
	progress.Text = answer.Text
	score, gs, gm := answer.Score, answer.GraderScore, answer.GraderMaxScore
	progress.Score = &score
	progress.GraderScore = &gs
	progress.GraderMaxScore = &gm
	progress.Verdict = answer.Verdict
	progress.CriterionResults = append([]diagnostic.CriterionResult(nil), answer.CriterionResults...)
	progress.Feedback = append([]string(nil), answer.Feedback...)
	if value.AcceptedRequests == nil {
		value.AcceptedRequests = map[string]diagnostic.AcceptedRequest{}
	}
	value.AcceptedRequests[key] = diagnostic.AcceptedRequest{Fingerprint: digest, Response: progress}
	if err = saveLocked(ctx, tx, value); err != nil {
		return diagnostic.Progress{}, err
	}
	raw, _ := json.Marshal(answer)
	pr, _ := json.Marshal(progress)
	order := len(value.Answers)
	if _, err = tx.Exec(ctx, `INSERT INTO diagnostic_answers(session_id,answer_order,idempotency_key,request_digest,variant_task_id,answer_data,progress_data) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, id, order, key, digest, answer.VariantTaskID, raw, pr); err != nil {
		return diagnostic.Progress{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return diagnostic.Progress{}, err
	}
	return progress, nil
}

func (s *Store) Fail(ctx context.Context, ownerID, id, token, transcriptionID, transcriptionText string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var data []byte
	if err = tx.QueryRow(ctx, `SELECT session_data FROM diagnostic_sessions WHERE owner_id=$1 AND id=$2 FOR UPDATE`, ownerID, id).Scan(&data); errors.Is(err, pgx.ErrNoRows) {
		return sessionNotFound()
	} else if err != nil {
		return err
	}
	value, err := decodeSession(data)
	if err != nil {
		return err
	}
	if value.InFlight == nil || value.InFlight.Token != token {
		return tx.Commit(ctx)
	}
	if transcriptionID != "" {
		value.InFlight.TranscriptionID = transcriptionID
		value.InFlight.TranscriptionText = transcriptionText
		value.InFlight.Token = ""
	} else {
		value.InFlight = nil
	}
	if err = saveLocked(ctx, tx, value); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type txer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func saveLocked(ctx context.Context, tx txer, value diagnostic.Session) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE diagnostic_sessions SET session_data=$2,status=$3,current_competency=$4,current_task=$5,updated_at=now(),completed_at=CASE WHEN $3='completed' THEN COALESCE(completed_at,now()) ELSE completed_at END WHERE id=$1`, value.ID, raw, value.Status, value.CurrentCompetency, value.CurrentTask)
	return err
}

func decodeSession(data []byte) (diagnostic.Session, error) {
	var value diagnostic.Session
	if err := json.Unmarshal(data, &value); err != nil {
		return diagnostic.Session{}, err
	}
	return value, nil
}

func startKeyConflict() error {
	return fault.New(fault.Conflict, "IDEMPOTENCY_KEY_REUSED", "Ключ запуска уже использован с другими параметрами.")
}

func variantNotFound() error {
	return fault.New(fault.NotFound, "VARIANT_NOT_FOUND", "Вариант не найден.")
}

func sessionNotFound() error {
	return fault.New(fault.NotFound, "DIAGNOSTIC_SESSION_NOT_FOUND", "Диагностическая сессия не найдена.")
}
