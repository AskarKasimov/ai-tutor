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
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: db.New(pool)}
}

type transaction struct{ queries *db.Queries }

func (r *Repository) Transaction(ctx context.Context, fn func(application.Transaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(&transaction{queries: r.queries.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (t *transaction) InsertUser(ctx context.Context, u user.User, hash string) error {
	err := t.queries.InsertUser(ctx, db.InsertUserParams{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, PasswordHash: hash, CreatedAt: u.CreatedAt})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fault.New(fault.Conflict, "EMAIL_ALREADY_REGISTERED", "Email уже зарегистрирован.")
	}
	return err
}

func (t *transaction) InsertSession(ctx context.Context, s session.Session) error {
	return t.queries.InsertAuthSession(ctx, db.InsertAuthSessionParams{ID: s.ID, UserID: s.UserID, ExpiresAt: s.ExpiresAt})
}

func (t *transaction) InsertAccess(ctx context.Context, hash []byte, id string, end int64) error {
	return t.queries.InsertAccessToken(ctx, db.InsertAccessTokenParams{TokenHash: hash, SessionID: id, ExpiresAt: end})
}

func (t *transaction) InsertRefresh(ctx context.Context, hash []byte, id string) error {
	return t.queries.InsertRefreshToken(ctx, db.InsertRefreshTokenParams{TokenHash: hash, SessionID: id})
}

func (t *transaction) LockRefresh(ctx context.Context, hash []byte) (session.RefreshState, bool, error) {
	row, err := t.queries.LockRefreshState(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return session.RefreshState{}, false, nil
	}
	if err != nil {
		return session.RefreshState{}, false, err
	}
	usedAt, err := t.queries.GetRefreshTokenUsedAt(ctx, hash)
	if err != nil {
		return session.RefreshState{}, false, err
	}
	return session.RefreshState{Session: session.Session{ID: row.ID, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt}, UsedAt: usedAt}, true, nil
}

func (t *transaction) MarkRefreshUsed(ctx context.Context, hash []byte, now int64) error {
	return t.queries.MarkRefreshTokenUsed(ctx, db.MarkRefreshTokenUsedParams{TokenHash: hash, UsedAt: &now})
}

func (t *transaction) RevokeSession(ctx context.Context, id string, now int64) error {
	return t.queries.RevokeAuthSession(ctx, db.RevokeAuthSessionParams{ID: id, RevokedAt: &now})
}

func (r *Repository) FindCredentials(ctx context.Context, email string) (user.User, string, error) {
	row, err := r.queries.GetUserForLogin(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, "", nil
	}
	return user.User{ID: row.ID, Email: row.Email, DisplayName: row.DisplayName, Role: user.Role(row.Role), CreatedAt: row.CreatedAt}, row.PasswordHash, err
}

func (r *Repository) FindAccess(ctx context.Context, hash []byte) (session.AccessState, user.User, bool, error) {
	row, err := r.queries.GetAuthContextByAccessToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return session.AccessState{}, user.User{}, false, nil
	}
	if err != nil {
		return session.AccessState{}, user.User{}, false, err
	}
	s := session.AccessState{Session: session.Session{ID: row.SessionID, UserID: row.UserID, ExpiresAt: row.SessionExpiresAt, RevokedAt: row.RevokedAt}, ExpiresAt: row.TokenExpiresAt}
	u := user.User{ID: row.AccountID, Email: row.Email, DisplayName: row.DisplayName, Role: user.Role(row.Role), CreatedAt: row.CreatedAt}
	return s, u, true, nil
}

func (r *Repository) RevokeByRefresh(ctx context.Context, hash []byte, now int64) error {
	return r.queries.RevokeSessionByRefreshToken(ctx, db.RevokeSessionByRefreshTokenParams{TokenHash: hash, RevokedAt: &now})
}
