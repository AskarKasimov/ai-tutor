package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

const password = "Надёжная фраза для теста 42!"

type fixture struct {
	app          *App
	pool         *pgxpool.Pool
	handler      http.Handler
	now          time.Time
	s3           *httptest.Server
	s3Mu         sync.Mutex
	s3Paths      []string
	s3AssetID    string
	s3Objects    map[string]fixtureS3Object
	s3HeadStatus int
}

type fixtureS3Object struct {
	data    []byte
	assetID string
}

func newFixture(t *testing.T) *fixture {
	return newFixtureWithConfig(t, testConfig())
}

func newFixtureWithConfig(t *testing.T, cfg Config) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pc.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subjects(id,name,created_at) VALUES ('subject:test','Тестовый предмет',1)`); err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: pool, now: time.Unix(1790762400, 0), s3Objects: make(map[string]fixtureS3Object)}
	f.s3 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.s3Mu.Lock()
		f.s3Paths = append(f.s3Paths, r.URL.Path)
		assetID := f.s3AssetID
		headStatus := f.s3HeadStatus
		object, exists := f.s3Objects[strings.TrimPrefix(r.URL.Path, "/task-audio-test/")]
		f.s3Mu.Unlock()
		switch r.Method {
		case http.MethodHead:
			if r.URL.Path == "/task-audio-test" {
				w.WriteHeader(http.StatusOK)
				return
			}
			if headStatus != 0 {
				w.WriteHeader(headStatus)
				return
			}
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(object.data)))
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("x-amz-meta-audio-asset-id", object.assetID)
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			key := strings.TrimPrefix(r.URL.Path, "/task-audio-test/")
			f.s3Mu.Lock()
			f.s3Objects[key] = fixtureS3Object{data: data, assetID: r.Header.Get("x-amz-meta-audio-asset-id")}
			f.s3Mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if !exists {
				w.Header().Set("Content-Type", "audio/wav")
				w.Header().Set("x-amz-meta-audio-asset-id", assetID)
				_, _ = w.Write(wavBytes())
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("x-amz-meta-audio-asset-id", object.assetID)
			w.Header().Set("Content-Length", strconv.Itoa(len(object.data)))
			_, _ = w.Write(object.data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(f.s3.Close)
	cfg.S3Endpoint = f.s3.URL
	cfg.S3CreateBucket = false
	a, err := New(cfg, pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	f.app = a
	a.now = func() time.Time { return f.now }
	f.handler = a.Handler()
	return f
}

func (f *fixture) request(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://api.example"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, req)
	return w
}

func (f *fixture) register(t *testing.T, email string) (*http.Cookie, *http.Cookie, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "display_name": "Иван"})
	w := f.request("POST", "/auth/register", string(body))
	if w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	cs := w.Result().Cookies()
	if len(cs) != 2 {
		t.Fatalf("cookies: %v", cs)
	}
	var result struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return cs[0], cs[1], result.User.ID
}

func requireCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var result struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != status || result.Code != code {
		t.Fatalf("got %d %s; want %d %s", w.Code, w.Body.String(), status, code)
	}
}

func TestAuthRegistrationCookiesNormalizationAndPersistence(t *testing.T) {
	f := newFixture(t)
	access, refresh, id := f.register(t, " Student@Example.edu ")
	for _, c := range []*http.Cookie{access, refresh} {
		if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Domain != "" {
			t.Fatalf("unsafe cookie: %s", c)
		}
	}
	if access.Path != "/" || access.MaxAge != 900 || refresh.Path != "/" || refresh.MaxAge != 2592000 {
		t.Fatal("wrong expiry/path")
	}
	w := f.request("GET", "/auth/me", "", access)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"email":"student@example.edu"`) || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	var hash string
	if err := f.pool.QueryRow(context.Background(), "SELECT password_hash FROM users WHERE id=$1", id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, password) {
		t.Fatal("password not hashed")
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE users SET role='admin' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(f.app.cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = f.app.now
	f.app = restarted
	f.handler = restarted.Handler()
	w = f.request("GET", "/auth/me", "", access)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"admin"`) {
		t.Fatalf("role/persistence: %s", w.Body.String())
	}
	requireCode(t, f.request("POST", "/auth/register", `{"email":"student@example.edu","password":"`+password+`"}`), 409, "EMAIL_ALREADY_REGISTERED")
}

func TestAuthLoginAndValidation(t *testing.T) {
	f := newFixture(t)
	f.register(t, "login@example.edu")
	bad := f.request("POST", "/auth/login", `{"email":"login@example.edu","password":"wrong"}`)
	unknown := f.request("POST", "/auth/login", `{"email":"unknown@example.edu","password":"wrong"}`)
	requireCode(t, bad, 401, "INVALID_CREDENTIALS")
	if bad.Body.String() != unknown.Body.String() {
		t.Fatal("credential enumeration")
	}
	good := f.request("POST", "/auth/login", `{"email":" LOGIN@example.edu ","password":"`+password+`"}`)
	if good.Code != 200 || len(good.Result().Cookies()) != 2 {
		t.Fatalf("login: %s", good.Body.String())
	}
	for _, body := range []string{
		`{"email":"x@example.edu","password":"` + password + `","role":"admin"}`,
		`{"email":"x@example.edu","password":"` + password + `","display_name":null}`,
		`{"email":"x@example.edu","password":"` + password + `","display_name":"bad\u0000name"}`,
		`{"email":"not-email","password":"` + password + `"}`,
		`{"email":"x@example.edu","password":"` + password + `"} {}`,
	} {
		requireCode(t, f.request("POST", "/auth/register", body), 422, "VALIDATION_ERROR")
	}
	for _, p := range []string{"short", strings.Repeat("a", 20), "passwordpassword", strings.Repeat("я", 129)} {
		b, _ := json.Marshal(map[string]string{"email": "weak@example.edu", "password": p})
		requireCode(t, f.request("POST", "/auth/register", string(b)), 422, "PASSWORD_TOO_WEAK")
	}
	requireCode(t, f.request("GET", "/auth/me", ""), 401, "UNAUTHORIZED")
}

func TestAuthRefreshReplayRevokesOnlyOneSession(t *testing.T) {
	f := newFixture(t)
	access, refresh, _ := f.register(t, "refresh@example.edu")
	login := f.request("POST", "/auth/login", `{"email":"refresh@example.edu","password":"`+password+`"}`)
	other := login.Result().Cookies()[0]
	f.now = f.now.Add(time.Minute)
	rotated := f.request("POST", "/auth/refresh", "", refresh)
	if rotated.Code != 200 {
		t.Fatalf("refresh: %s", rotated.Body.String())
	}
	next := rotated.Result().Cookies()
	if next[1].Value == refresh.Value || next[1].MaxAge != 2591940 {
		t.Fatal("refresh not rotated/fixed end lost")
	}
	if f.request("GET", "/auth/me", "", access).Code != 200 {
		t.Fatal("old access invalidated too early")
	}
	replay := f.request("POST", "/auth/refresh", "", refresh)
	requireCode(t, replay, 401, "INVALID_REFRESH_TOKEN")
	for _, c := range replay.Result().Cookies() {
		if c.MaxAge != -1 || c.Value != "" {
			t.Fatal("cookie not cleared")
		}
	}
	requireCode(t, f.request("GET", "/auth/me", "", access), 401, "UNAUTHORIZED")
	requireCode(t, f.request("GET", "/auth/me", "", next[0]), 401, "UNAUTHORIZED")
	if f.request("GET", "/auth/me", "", other).Code != 200 {
		t.Fatal("other session revoked")
	}
}

func TestAuthLogoutAndExpiry(t *testing.T) {
	f := newFixture(t)
	access, refresh, _ := f.register(t, "logout@example.edu")
	if f.request("POST", "/auth/logout", "", refresh).Code != 204 {
		t.Fatal("logout failed")
	}
	requireCode(t, f.request("GET", "/auth/me", "", access), 401, "UNAUTHORIZED")
	if f.request("POST", "/auth/logout", "").Code != 204 {
		t.Fatal("logout not idempotent")
	}
	access, refresh, _ = f.register(t, "expiry@example.edu")
	f.now = f.now.Add(900 * time.Second)
	requireCode(t, f.request("GET", "/auth/me", "", access), 401, "UNAUTHORIZED")
	if f.request("POST", "/auth/refresh", "", refresh).Code != 200 {
		t.Fatal("valid refresh rejected")
	}
	f.now = f.now.Add(30 * 24 * time.Hour)
	requireCode(t, f.request("POST", "/auth/refresh", "", refresh), 401, "INVALID_REFRESH_TOKEN")
}

func TestAuthConcurrentRefresh(t *testing.T) {
	f := newFixture(t)
	access, refresh, _ := f.register(t, "concurrent@example.edu")
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- f.request("POST", "/auth/refresh", "", refresh).Code }()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[401] != 1 {
		t.Fatalf("rotation race: %v", counts)
	}
	requireCode(t, f.request("GET", "/auth/me", "", access), 401, "UNAUTHORIZED")
}

func TestRepeatedLoginFailuresDoNotBlockAuthentication(t *testing.T) {
	f := newFixture(t)
	for range 12 {
		w := f.request("POST", "/auth/login", `{"email":"login@example.edu","password":"wrong"}`)
		requireCode(t, w, 401, "INVALID_CREDENTIALS")
		if w.Header().Get("Retry-After") != "" {
			t.Fatal("unexpected retry delay")
		}
	}
	var exists bool
	if err := f.pool.QueryRow(context.Background(), "SELECT to_regclass('auth_rate_limits') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatalf("obsolete counter table: exists=%v, err=%v", exists, err)
	}
}

type noNetworkTransport struct{ t *testing.T }

func (transport noNetworkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Error("registration made an outgoing HTTP request")
	return nil, errors.New("network disabled")
}

func TestRegistrationHasNoExternalPasswordDependency(t *testing.T) {
	f := newFixture(t)
	f.app.client.Transport = noNetworkTransport{t}
	access, _, _ := f.register(t, "student@example.edu")
	w := f.request("GET", "/auth/me", "", access)
	if w.Code != 200 {
		t.Fatalf("session: %d %s", w.Code, w.Body.String())
	}
}

func TestAuthPostRequestsHaveNoSharedIPLimit(t *testing.T) {
	f := newFixture(t)
	for range 35 {
		requireCode(t, f.request("POST", "/auth/register", `{}`), 422, "VALIDATION_ERROR")
		requireCode(t, f.request("POST", "/auth/login", `{}`), 422, "VALIDATION_ERROR")
		requireCode(t, f.request("POST", "/auth/refresh", ""), 401, "INVALID_REFRESH_TOKEN")
		if w := f.request("POST", "/auth/logout", ""); w.Code != 204 {
			t.Fatalf("logout: %d", w.Code)
		}
	}
}
