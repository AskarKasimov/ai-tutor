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
