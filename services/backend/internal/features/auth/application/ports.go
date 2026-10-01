package application

import (
	"context"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
)

type PasswordHasher interface {
	Hash(context.Context, string) (string, error)
	Verify(context.Context, string, string) (bool, error)
}
type BreachChecker interface {
	Compromised(context.Context, string) (bool, error)
}
type Transaction interface {
	InsertUser(context.Context, user.User, string) error
	InsertSession(context.Context, session.Session) error
	InsertAccess(context.Context, []byte, string, int64) error
	InsertRefresh(context.Context, []byte, string) error
	LockRefresh(context.Context, []byte) (session.RefreshState, bool, error)
	MarkRefreshUsed(context.Context, []byte, int64) error
	RevokeSession(context.Context, string, int64) error
}
type Repository interface {
	Transaction(context.Context, func(Transaction) error) error
	FindCredentials(context.Context, string) (user.User, string, error)
	FindAccess(context.Context, []byte) (session.AccessState, user.User, bool, error)
	RevokeByRefresh(context.Context, []byte, int64) error
	RateCounter(ctx context.Context, key string, now, windowSeconds int64) (int, int64, error)
}
