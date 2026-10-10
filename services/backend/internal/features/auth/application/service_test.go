package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type credentials struct {
	u    user.User
	hash string
}
type refreshEntry struct {
	id   string
	used *int64
}
type accessEntry struct {
	id  string
	end int64
}
type memoryRepo struct {
	users     map[string]credentials
	sessions  map[string]session.Session
	refresh   map[string]refreshEntry
	access    map[string]accessEntry
	commits   int
	commitErr error
}

func newMemory() *memoryRepo {
	return &memoryRepo{users: map[string]credentials{}, sessions: map[string]session.Session{}, refresh: map[string]refreshEntry{}, access: map[string]accessEntry{}}
}
func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V)
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (m *memoryRepo) Transaction(ctx context.Context, fn func(Transaction) error) error {
	copy := *m
	copy.users = cloneMap(m.users)
	copy.sessions = cloneMap(m.sessions)
	copy.refresh = cloneMap(m.refresh)
	copy.access = cloneMap(m.access)
	if err := fn(&copy); err != nil {
		return err
	}
	if m.commitErr != nil {
		return m.commitErr
	}
	copy.commits++
	*m = copy
	return nil
}
func (m *memoryRepo) InsertUser(_ context.Context, u user.User, hash string) error {
	if _, ok := m.users[u.Email]; ok {
		return fault.New(fault.Conflict, "EMAIL_ALREADY_REGISTERED", "")
	}
	m.users[u.Email] = credentials{u, hash}
	return nil
}

func (m *memoryRepo) EnsureTeacher(_ context.Context, now int64) (user.User, error) {
	if existing, ok := m.users[TeacherEmail]; ok {
		return existing.u, nil
	}
	name := "Учитель"
	u := user.User{ID: "user:shared-teacher", Email: TeacherEmail, DisplayName: &name, Role: user.Admin, CreatedAt: now}
	m.users[TeacherEmail] = credentials{u, "!"}
	return u, nil
}
func (m *memoryRepo) InsertSession(_ context.Context, s session.Session) error {
	m.sessions[s.ID] = s
	return nil
}
func (m *memoryRepo) InsertAccess(_ context.Context, hash []byte, id string, end int64) error {
	m.access[string(hash)] = accessEntry{id, end}
	return nil
}
func (m *memoryRepo) InsertRefresh(_ context.Context, hash []byte, id string) error {
	m.refresh[string(hash)] = refreshEntry{id, nil}
	return nil
}
func (m *memoryRepo) LockRefresh(_ context.Context, hash []byte) (session.RefreshState, bool, error) {
	v, ok := m.refresh[string(hash)]
	return session.RefreshState{Session: m.sessions[v.id], UsedAt: v.used}, ok, nil
}
func (m *memoryRepo) MarkRefreshUsed(_ context.Context, hash []byte, now int64) error {
	v := m.refresh[string(hash)]
	v.used = &now
	m.refresh[string(hash)] = v
	return nil
}
func (m *memoryRepo) RevokeSession(_ context.Context, id string, now int64) error {
	s := m.sessions[id]
	if s.RevokedAt == nil {
		s.RevokedAt = &now
	}
	m.sessions[id] = s
	return nil
}
func (m *memoryRepo) FindCredentials(_ context.Context, email string) (user.User, string, error) {
	v := m.users[email]
	return v.u, v.hash, nil
}
func (m *memoryRepo) FindAccess(_ context.Context, hash []byte) (session.AccessState, user.User, bool, error) {
	v, ok := m.access[string(hash)]
	s := m.sessions[v.id]
	var u user.User
	for _, c := range m.users {
		if c.u.ID == s.UserID {
			u = c.u
		}
	}
	return session.AccessState{Session: s, ExpiresAt: v.end}, u, ok, nil
}
func (m *memoryRepo) RevokeByRefresh(ctx context.Context, hash []byte, now int64) error {
	v, ok := m.refresh[string(hash)]
	if !ok {
		return nil
	}
	return m.RevokeSession(ctx, v.id, now)
}

type hasher struct{ verified string }

func (h *hasher) Hash(_ context.Context, p string) (string, error) { return "hash:" + p, nil }
func (h *hasher) Verify(_ context.Context, hash, p string) (bool, error) {
	h.verified = hash
	return hash != "" && hash == "hash:"+p, nil
}

func fixture() (*Service, *memoryRepo, *time.Time) {
	now := time.Unix(1000, 0)
	m := newMemory()
	s := New(m, &hasher{}, func() time.Time { return now })
	return s, m, &now
}

const phrase = "Надёжная фраза для теста 42!"

func register(t *testing.T, s *Service, email string) AuthResult {
	t.Helper()
	out, err := s.Register(context.Background(), RegisterInput{Email: email, Password: phrase, DisplayName: ptr("Иван")})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var e *fault.Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("got %v, want %s", err, want)
	}
}
func TestRegistrationPolicyAndNormalization(t *testing.T) {
	s, m, _ := fixture()
	ctx := context.Background()
	for _, p := range []string{"short", strings.Repeat("a", 20), "passwordpassword", strings.Repeat("я", 129)} {
		_, err := s.Register(ctx, RegisterInput{Email: "x@example.edu", Password: p, DisplayName: ptr("Иван")})
		code(t, err, "PASSWORD_TOO_WEAK")
	}
	if len(m.users) != 0 {
		t.Fatal("weak passwords reached dependencies")
	}
	out := register(t, s, " Student@Example.edu ")
	if out.User.Email != "student@example.edu" || out.User.Role != user.Student || out.Tokens.Expiry.AccessExpiresAt != 1900 || out.Tokens.Expiry.RefreshExpiresAt != 2593000 {
		t.Fatalf("registration: %+v", out)
	}
	_, err := s.Register(ctx, RegisterInput{Email: "student@example.edu", Password: phrase, DisplayName: ptr("Иван")})
	code(t, err, "EMAIL_ALREADY_REGISTERED")
	if len(m.sessions) != 1 {
		t.Fatal("duplicate registration created session")
	}
}

func TestRefreshFixedEndOldAccessAndCommittedReplay(t *testing.T) {
	s, m, now := fixture()
	ctx := context.Background()
	first := register(t, s, "a@example.edu")
	other, err := s.Login(ctx, "a@example.edu", phrase)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	next, err := s.Refresh(ctx, first.Tokens.Refresh)
	if err != nil {
		t.Fatal(err)
	}
	if next.Refresh == first.Tokens.Refresh || next.Expiry.RefreshExpiresAt != first.Tokens.Expiry.RefreshExpiresAt || next.Expiry.AccessExpiresAt != now.Unix()+900 {
		t.Fatal("rotation did not preserve fixed end")
	}
	if _, err = s.Me(ctx, first.Tokens.Access); err != nil {
		t.Fatal("old access invalidated", err)
	}
	commits := m.commits
	_, err = s.Refresh(ctx, first.Tokens.Refresh)
	code(t, err, "INVALID_REFRESH_TOKEN")
	if m.commits != commits+1 {
		t.Fatal("replay revocation did not commit")
	}
	for _, token := range []string{first.Tokens.Access, next.Access} {
		_, err = s.Me(ctx, token)
		code(t, err, "UNAUTHORIZED")
	}
	if _, err = s.Me(ctx, other.Tokens.Access); err != nil {
		t.Fatal("other session revoked", err)
	}
}
func TestReplayCommitFailureReturned(t *testing.T) {
	s, m, _ := fixture()
	out := register(t, s, "a@example.edu")
	ctx := context.Background()
	if _, err := s.Refresh(ctx, out.Tokens.Refresh); err != nil {
		t.Fatal(err)
	}
	m.commitErr = errors.New("commit failed")
	_, err := s.Refresh(ctx, out.Tokens.Refresh)
	if !errors.Is(err, m.commitErr) {
		t.Fatalf("got %v", err)
	}
	if _, err = s.Me(ctx, out.Tokens.Access); err != nil {
		t.Fatal("failed commit changed session")
	}
}
func TestExpiryBoundariesAndLogout(t *testing.T) {
	s, _, now := fixture()
	ctx := context.Background()
	out := register(t, s, "a@example.edu")
	*now = now.Add(900 * time.Second)
	_, err := s.Me(ctx, out.Tokens.Access)
	code(t, err, "UNAUTHORIZED")
	*now = time.Unix(out.Tokens.Expiry.RefreshExpiresAt-10, 0)
	next, err := s.Refresh(ctx, out.Tokens.Refresh)
	if err != nil {
		t.Fatal(err)
	}
	if next.Expiry.AccessExpiresAt != out.Tokens.Expiry.RefreshExpiresAt {
		t.Fatal("access outlives session")
	}
	*now = now.Add(10 * time.Second)
	_, err = s.Refresh(ctx, next.Refresh)
	code(t, err, "INVALID_REFRESH_TOKEN")
	if err = s.Logout(ctx, next.Refresh); err != nil {
		t.Fatal(err)
	}
	if err = s.Logout(ctx, ""); err != nil {
		t.Fatal(err)
	}
}
func TestLoginAttemptsDoNotBlockLaterAuthentication(t *testing.T) {
	s, _, _ := fixture()
	ctx := context.Background()
	register(t, s, "login@example.edu")
	for range 120 {
		_, err := s.Login(ctx, "login@example.edu", "wrong")
		code(t, err, "INVALID_CREDENTIALS")
	}
	if _, err := s.Login(ctx, "login@example.edu", phrase); err != nil {
		t.Fatal(err)
	}
	h := &hasher{verified: "sentinel"}
	s.hasher = h
	_, err := s.Login(ctx, "unknown@example.edu", "wrong")
	code(t, err, "INVALID_CREDENTIALS")
	if h.verified != "" {
		t.Fatal("unknown user skipped verifier")
	}
}

func ptr(s string) *string { return &s }
