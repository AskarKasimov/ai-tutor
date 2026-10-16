// Package postgres stores Geek diagnostics and their idempotent requests.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func (s *Store) SubjectExists(ctx context.Context, subjectID string) (bool, error) {
	ok, err := db.New(s.pool).GeekSubjectExists(ctx, subjectID)
	if err != nil {
		return false, fmt.Errorf("check subject %s: %w", subjectID, err)
	}
	return ok, nil
}

func (s *Store) Session(ctx context.Context, ownerID, id string) (exam.Session, error) {
	state, err := db.New(s.pool).GetGeekSession(ctx, db.GetGeekSessionParams{ID: id, OwnerID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return exam.Session{}, application.ErrNotFound
	}
	if err != nil {
		return exam.Session{}, fmt.Errorf("get geek session %s: %w", id, err)
	}
	return decode(state)
}

func (s *Store) ActiveSession(ctx context.Context, ownerID, subjectID string) (exam.Session, bool, error) {
	state, err := db.New(s.pool).GetActiveGeekSession(ctx, db.GetActiveGeekSessionParams{OwnerID: ownerID, SubjectID: subjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return exam.Session{}, false, nil
	}
	if err != nil {
		return exam.Session{}, false, fmt.Errorf("get active geek session: %w", err)
	}
	session, err := decode(state)
	return session, err == nil, err
}

func (s *Store) Request(ctx context.Context, ownerID, key string) (application.Request, bool, error) {
	row, err := db.New(s.pool).GetGeekRequest(ctx, db.GetGeekRequestParams{OwnerID: ownerID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.Request{}, false, nil
	}
	if err != nil {
		return application.Request{}, false, fmt.Errorf("get geek request: %w", err)
	}
	response, err := decode(row.Response)
	if err != nil {
		return application.Request{}, false, err
	}
	return application.Request{OwnerID: ownerID, Key: key, Fingerprint: row.Fingerprint, SessionID: row.SessionID, Created: row.Created, Response: response, CreatedAt: row.CreatedAt}, true, nil
}

func (s *Store) Create(ctx context.Context, session exam.Session, r application.Request) error {
	return s.tx(ctx, func(q *db.Queries) error {
		state, err := json.Marshal(session)
		if err != nil {
			return err
		}
		err = q.InsertGeekSession(ctx, db.InsertGeekSessionParams{
			ID: session.ID, OwnerID: session.OwnerID, SubjectID: session.SubjectID, Status: string(session.Status),
			Version: int32(session.Version), StartedAt: session.StartedAt, CompletedAt: session.CompletedAt,
			UpdatedAt: s.now().Unix(), State: state,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "geek_sessions_one_active" {
			return application.ErrActiveExists
		}
		if err != nil {
			return fmt.Errorf("insert geek session: %w", err)
		}
		return insertRequest(ctx, q, r)
	})
}

func (s *Store) Update(ctx context.Context, session exam.Session, previous int, r *application.Request) error {
	return s.tx(ctx, func(q *db.Queries) error {
		state, err := json.Marshal(session)
		if err != nil {
			return err
		}
		rows, err := q.UpdateGeekSession(ctx, db.UpdateGeekSessionParams{
			ID: session.ID, PreviousVersion: int32(previous), Status: string(session.Status), Version: int32(session.Version),
			CompletedAt: session.CompletedAt, UpdatedAt: s.now().Unix(), State: state,
		})
		if err != nil {
			return fmt.Errorf("update geek session: %w", err)
		}
		if rows == 0 {
			return application.ErrConflict
		}
		if r == nil {
			return nil
		}
		return insertRequest(ctx, q, *r)
	})
}

func (s *Store) SaveRequest(ctx context.Context, r application.Request) error {
	return insertRequest(ctx, db.New(s.pool), r)
}

func (s *Store) tx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin geek transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertRequest(ctx context.Context, q *db.Queries, r application.Request) error {
	response, err := json.Marshal(r.Response)
	if err != nil {
		return err
	}
	err = q.InsertGeekRequest(ctx, db.InsertGeekRequestParams{
		OwnerID: r.OwnerID, Key: r.Key, Fingerprint: r.Fingerprint, SessionID: r.SessionID,
		Created: r.Created, Response: response, CreatedAt: r.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("insert geek request: %w", err)
	}
	return nil
}

func decode(state []byte) (exam.Session, error) {
	var session exam.Session
	if err := json.Unmarshal(state, &session); err != nil {
		return exam.Session{}, fmt.Errorf("decode geek session: %w", err)
	}
	return session, nil
}

// Transcriptions reads the student's own confirmed transcriptions.
type Transcriptions struct{ pool *pgxpool.Pool }

func NewTranscriptions(pool *pgxpool.Pool) *Transcriptions { return &Transcriptions{pool: pool} }

func (t *Transcriptions) Text(ctx context.Context, ownerID, id string) (string, bool, error) {
	text, err := db.New(t.pool).GetOwnedTranscriptionText(ctx, db.GetOwnedTranscriptionTextParams{ID: id, UserID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get transcription: %w", err)
	}
	return text, true, nil
}
