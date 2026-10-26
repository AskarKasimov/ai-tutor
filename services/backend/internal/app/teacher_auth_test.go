package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// portal sends a request from the admin panel.
func (f *fixture) portal(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://api.example"+examTestPath(path), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(httpx.PortalHeader, "admin")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, req)
	return w
}

func TestTeacherLoginAndSessions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.handler = f.app.Handler()
	// The student site cannot open the teacher account.
	requireCode(t, f.request("POST", "/auth/teacher", ""), 404, "NOT_FOUND")
	login := f.portal("POST", "/auth/teacher", "")
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var body struct {
		User user.User `json:"user"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.User.Role != user.Admin || body.User.ID == "" {
		t.Fatalf("teacher: %+v", body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatal("missing session cookies")
	}
	for _, cookie := range cookies {
		if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatal("invalid session cookie")
		}
	}
	access, refresh := cookies[0], cookies[1]
	if access.Name != "admin_access_token" || refresh.Name != "admin_refresh_token" {
		t.Fatalf("admin cookies: %s %s", access.Name, refresh.Name)
	}
	second := f.portal("POST", "/auth/teacher", "")
	var repeated struct {
		User user.User `json:"user"`
	}
	json.Unmarshal(second.Body.Bytes(), &repeated)
	if repeated.User.ID != body.User.ID || cookies[0].Value == second.Result().Cookies()[0].Value {
		t.Fatal("teacher identity/session isolation")
	}
	f.handler = f.app.Handler()
	if got := f.portal("GET", "/auth/me", "", access); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	created := f.portal("POST", "/admin/subjects", `{"name":"Предмет учителя"}`, access)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var subject struct {
		ID string `json:"id"`
	}
	json.Unmarshal(created.Body.Bytes(), &subject)
	if materials := f.portal("GET", "/admin/subjects/"+subject.ID+"/materials", "", access); materials.Code != 200 {
		t.Fatal(materials.Body.String())
	}
	if got := f.portal("POST", "/auth/refresh", "", refresh); got.Code != 200 {
		t.Fatal(got.Body.String())
	} else {
		refresh = got.Result().Cookies()[1]
	}
	if got := f.portal("POST", "/auth/logout", "", refresh); got.Code != 204 {
		t.Fatal(got.Body.String())
	}
	requireCode(t, f.portal("GET", "/auth/me", "", access), 401, "UNAUTHORIZED")
	if got := f.portal("GET", "/auth/me", "", second.Result().Cookies()[0]); got.Code != 200 {
		t.Fatal("logout revoked another teacher session")
	}
	requireCode(t, f.portal("POST", "/auth/teacher", `{}`), 422, "VALIDATION_ERROR")
	requireCode(t, f.request("POST", "/auth/register", `{"email":"shared-teacher@ai-tutor.invalid","password":"`+password+`","display_name":"Учитель"}`), 422, "VALIDATION_ERROR")
}

// One browser keeps a student session and an admin panel session at once:
// each portal reads only its own cookies.
func TestAdminPortalSessionIsIndependentFromStudent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	studentAccess, studentRefresh, studentID := f.register(t, "student@example.edu")
	login := f.portal("POST", "/auth/teacher", "")
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	adminAccess, adminRefresh := login.Result().Cookies()[0], login.Result().Cookies()[1]
	all := []*http.Cookie{studentAccess, studentRefresh, adminAccess, adminRefresh}
	me := func(w *httptest.ResponseRecorder) user.User {
		t.Helper()
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var u user.User
		_ = json.Unmarshal(w.Body.Bytes(), &u)
		return u
	}
	if u := me(f.request("GET", "/auth/me", "", all...)); u.ID != studentID || u.Role != user.Student {
		t.Fatalf("student site sees %+v", u)
	}
	if u := me(f.portal("GET", "/auth/me", "", all...)); u.Role != user.Admin {
		t.Fatalf("admin panel sees %+v", u)
	}
	// The student cookie alone does not open the admin panel.
	requireCode(t, f.portal("GET", "/auth/me", "", studentAccess), 401, "UNAUTHORIZED")
	// Signing out of the admin panel keeps the student signed in.
	if got := f.portal("POST", "/auth/logout", "", all...); got.Code != 204 {
		t.Fatal(got.Body.String())
	}
	if u := me(f.request("GET", "/auth/me", "", all...)); u.ID != studentID {
		t.Fatalf("student session lost: %+v", u)
	}
}

func TestConcurrentTeacherLoginUsesOneAccount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := f.portal("POST", "/auth/teacher", ""); w.Code != 200 {
				t.Errorf("login: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM users WHERE role='admin'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("teachers=%d err=%v", count, err)
	}
}
