package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool} }

type transaction struct{ tx pgx.Tx }

func (r *Repository) Transaction(ctx context.Context, fn func(application.Transaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(&transaction{tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (t *transaction) InsertUser(ctx context.Context, u user.User, hash string) error {
	_, err := t.tx.Exec(ctx, "INSERT INTO users(id,email,display_name,password_hash,created_at) VALUES($1,$2,$3,$4,$5)", u.ID, u.Email, u.DisplayName, hash, u.CreatedAt)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return fault.New(fault.Conflict, "EMAIL_ALREADY_REGISTERED", "Email уже зарегистрирован.")
	}
	return err
}
func (t *transaction) InsertSession(ctx context.Context, s session.Session) error {
	_, err := t.tx.Exec(ctx, "INSERT INTO auth_sessions(id,user_id,expires_at) VALUES($1,$2,$3)", s.ID, s.UserID, s.ExpiresAt)
	return err
}
func (t *transaction) InsertAccess(ctx context.Context, hash []byte, id string, end int64) error {
	_, err := t.tx.Exec(ctx, "INSERT INTO access_tokens(token_hash,session_id,expires_at) VALUES($1,$2,$3)", hash, id, end)
	return err
}
func (t *transaction) InsertRefresh(ctx context.Context, hash []byte, id string) error {
	_, err := t.tx.Exec(ctx, "INSERT INTO refresh_tokens(token_hash,session_id) VALUES($1,$2)", hash, id)
	return err
}
func (t *transaction) LockRefresh(ctx context.Context, hash []byte) (session.RefreshState, bool, error) {
	var state session.RefreshState
	err := t.tx.QueryRow(ctx, `SELECT s.id,s.expires_at,s.revoked_at,t.used_at FROM refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id WHERE t.token_hash=$1 FOR UPDATE OF s`, hash).Scan(&state.Session.ID, &state.Session.ExpiresAt, &state.Session.RevokedAt, &state.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	// A waiting SELECT can retain a token snapshot from before the session lock.
	// Reread under the acquired lock so concurrent exchanges observe consumption.
	err = t.tx.QueryRow(ctx, "SELECT used_at FROM refresh_tokens WHERE token_hash=$1", hash).Scan(&state.UsedAt)
	return state, err == nil, err
}
func (t *transaction) MarkRefreshUsed(ctx context.Context, hash []byte, now int64) error {
	_, err := t.tx.Exec(ctx, "UPDATE refresh_tokens SET used_at=$2 WHERE token_hash=$1", hash, now)
	return err
}
func (t *transaction) RevokeSession(ctx context.Context, id string, now int64) error {
	_, err := t.tx.Exec(ctx, "UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE id=$1", id, now)
	return err
}
func (r *Repository) FindCredentials(ctx context.Context, email string) (user.User, string, error) {
	var u user.User
	var hash string
	err := r.pool.QueryRow(ctx, "SELECT id,email,display_name,role,created_at,password_hash FROM users WHERE email=$1", email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return u, hash, err
}
func (r *Repository) FindAccess(ctx context.Context, hash []byte) (session.AccessState, user.User, bool, error) {
	var s session.AccessState
	var u user.User
	err := r.pool.QueryRow(ctx, `SELECT s.id,s.user_id,s.expires_at,s.revoked_at,t.expires_at,u.id,u.email,u.display_name,u.role,u.created_at FROM access_tokens t JOIN auth_sessions s ON s.id=t.session_id JOIN users u ON u.id=s.user_id WHERE t.token_hash=$1`, hash).Scan(&s.Session.ID, &s.Session.UserID, &s.Session.ExpiresAt, &s.Session.RevokedAt, &s.ExpiresAt, &u.ID, &u.Email, &u.DisplayName, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, u, false, nil
	}
	return s, u, err == nil, err
}
func (r *Repository) RevokeByRefresh(ctx context.Context, hash []byte, now int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE id=(SELECT session_id FROM refresh_tokens WHERE token_hash=$1)`, hash, now)
	return err
}
