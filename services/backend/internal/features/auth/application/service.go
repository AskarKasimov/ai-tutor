package application

import (
	"context"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type RegisterInput struct {
	Email, Password string
	DisplayName     *string
}
type AuthResult struct {
	User   user.User
	Tokens session.Tokens
}
type Service struct {
	repo   Repository
	hasher PasswordHasher
	now    func() time.Time
}

func New(repo Repository, hasher PasswordHasher, now func() time.Time) *Service {
	return &Service{repo: repo, hasher: hasher, now: now}
}
func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || !strings.Contains(email, "@") || strings.ContainsAny(email, "\r\n") {
		return "", fault.Validation("email", "Укажите корректный email до 254 символов.")
	}
	return email, nil
}
func weakPassword(message string) *fault.Error {
	e := fault.New(fault.Invalid, "PASSWORD_TOO_WEAK", "Пароль не соответствует политике.")
	e.Details = []fault.Detail{{Path: "password", Code: "PASSWORD_TOO_WEAK", Message: message}}
	return e
}
func checkPassword(password string) error {
	count := utf8.RuneCountInString(password)
	if count < 15 || count > 128 {
		return weakPassword("Пароль должен содержать 15–128 символов.")
	}
	p := strings.ToLower(password)
	repeated := true
	var first rune
	for i, r := range p {
		if i == 0 {
			first = r
		} else if r != first {
			repeated = false
		}
	}
	if repeated || strings.TrimSpace(p) == "" || p == "passwordpassword" || p == "123456789012345" || p == "qwertyuiopasdfgh" {
		return weakPassword("Выберите менее распространённый пароль.")
	}

	return nil
}
func (s *Service) Register(ctx context.Context, in RegisterInput) (AuthResult, error) {
	var out AuthResult
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return out, err
	}
	if in.DisplayName != nil {
		n := *in.DisplayName
		if utf8.RuneCountInString(n) < 1 || utf8.RuneCountInString(n) > 200 || strings.TrimSpace(n) == "" || strings.IndexByte(n, 0) >= 0 {
			return out, fault.Validation("display_name", "Имя должно содержать 1–200 символов.")
		}
	}
	if err = checkPassword(in.Password); err != nil {
		return out, err
	}
	hash, err := s.hasher.Hash(ctx, in.Password)
	if err != nil {
		return out, err
	}
	now := s.now().Unix()
	id, err := security.ID("user")
	if err != nil {
		return out, err
	}
	out.User = user.User{ID: id, Email: email, DisplayName: in.DisplayName, Role: user.Student, CreatedAt: now}
	err = s.repo.Transaction(ctx, func(tx Transaction) error {
		if err := tx.InsertUser(ctx, out.User, hash); err != nil {
			return err
		}
		var err error
		out.Tokens, err = s.newSession(ctx, tx, id, now)
		return err
	})
	return out, err
}
func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	var out AuthResult
	email, err := normalizeEmail(email)
	if err != nil {
		return out, err
	}
	if n := utf8.RuneCountInString(password); n < 1 || n > 128 {
		return out, fault.Validation("password", "Пароль должен содержать 1–128 символов.")
	}
	u, hash, err := s.repo.FindCredentials(ctx, email)
	if err != nil {
		return out, err
	}
	ok, err := s.hasher.Verify(ctx, hash, password)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, fault.New(fault.Unauthorized, "INVALID_CREDENTIALS", "Email или пароль неверен.")
	}
	out.User = u
	err = s.repo.Transaction(ctx, func(tx Transaction) error {
		var err error
		out.Tokens, err = s.newSession(ctx, tx, u.ID, s.now().Unix())
		return err
	})
	return out, err
}
func (s *Service) newSession(ctx context.Context, tx Transaction, userID string, now int64) (session.Tokens, error) {
	id, err := security.ID("auth")
	if err != nil {
		return session.Tokens{}, err
	}
	end := now + 2592000
	if err = tx.InsertSession(ctx, session.Session{ID: id, UserID: userID, ExpiresAt: end}); err != nil {
		return session.Tokens{}, err
	}
	return s.issueTokens(ctx, tx, id, end, now)
}
func (s *Service) issueTokens(ctx context.Context, tx Transaction, id string, end, now int64) (session.Tokens, error) {
	var t session.Tokens
	var err error
	t.Access, err = security.Token()
	if err != nil {
		return t, err
	}
	t.Refresh, err = security.Token()
	if err != nil {
		return t, err
	}
	t.Expiry = session.Expiry{AccessExpiresAt: min(now+900, end), RefreshExpiresAt: end}
	if err = tx.InsertAccess(ctx, security.Hash(t.Access), id, t.Expiry.AccessExpiresAt); err != nil {
		return t, err
	}
	err = tx.InsertRefresh(ctx, security.Hash(t.Refresh), id)
	return t, err
}
func invalidRefresh() *fault.Error {
	return fault.New(fault.Unauthorized, "INVALID_REFRESH_TOKEN", "Refresh-токен недействителен.")
}
func (s *Service) Refresh(ctx context.Context, token string) (session.Tokens, error) {
	var out session.Tokens
	if token == "" {
		return out, invalidRefresh()
	}
	replay := false
	err := s.repo.Transaction(ctx, func(tx Transaction) error {
		state, found, err := tx.LockRefresh(ctx, security.Hash(token))
		if err != nil {
			return err
		}
		if !found {
			return invalidRefresh()
		}
		now := s.now().Unix()
		if state.UsedAt != nil {
			if err = tx.RevokeSession(ctx, state.Session.ID, now); err != nil {
				return err
			}
			replay = true
			return nil
		}
		if state.Session.RevokedAt != nil || state.Session.ExpiresAt <= now {
			return invalidRefresh()
		}
		if err = tx.MarkRefreshUsed(ctx, security.Hash(token), now); err != nil {
			return err
		}
		out, err = s.issueTokens(ctx, tx, state.Session.ID, state.Session.ExpiresAt, now)
		return err
	})
	if err != nil {
		return out, err
	}
	if replay {
		return out, invalidRefresh()
	}
	return out, nil
}
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.RevokeByRefresh(ctx, security.Hash(token), s.now().Unix())
}
func (s *Service) Me(ctx context.Context, token string) (user.User, error) {
	unauthorized := func() (user.User, error) {
		return user.User{}, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия.")
	}
	if token == "" {
		return unauthorized()
	}
	state, u, found, err := s.repo.FindAccess(ctx, security.Hash(token))
	if err != nil {
		return user.User{}, err
	}
	now := s.now().Unix()
	if !found || state.ExpiresAt <= now || state.Session.ExpiresAt <= now || state.Session.RevokedAt != nil {
		return unauthorized()
	}
	return u, nil
}
