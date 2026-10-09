package app

import (
	"encoding/json"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"net/http"
	"sync"
	"testing"
)

func TestTeacherLoginAcrossModesAndSessions(t *testing.T) {
	for _, mode := range []string{"real", "mock"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.app.cfg.APIMode = mode
			f.handler = f.app.Handler()
			login := f.request("POST", "/auth/teacher", "")
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
			second := f.request("POST", "/auth/teacher", "")
			var repeated struct {
				User user.User `json:"user"`
			}
			json.Unmarshal(second.Body.Bytes(), &repeated)
			if repeated.User.ID != body.User.ID || cookies[0].Value == second.Result().Cookies()[0].Value {
				t.Fatal("teacher identity/session isolation")
			}
			f.handler = f.app.Handler()
			if got := f.request("GET", "/auth/me", "", access); got.Code != 200 {
				t.Fatal(got.Body.String())
			}
			created := f.request("POST", "/admin/subjects", `{"name":"Предмет учителя"}`, access)
			if created.Code != 201 {
				t.Fatal(created.Body.String())
			}
			var subject struct {
				ID string `json:"id"`
			}
			json.Unmarshal(created.Body.Bytes(), &subject)
			imported := upload(f, "/admin/subjects/"+subject.ID+"/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), access)
			if imported.Code != 200 {
				t.Fatal(imported.Body.String())
			}
			if got := f.request("POST", "/auth/refresh", "", refresh); got.Code != 200 {
				t.Fatal(got.Body.String())
			} else {
				refresh = got.Result().Cookies()[1]
			}
			if got := f.request("POST", "/auth/logout", "", refresh); got.Code != 204 {
				t.Fatal(got.Body.String())
			}
			requireCode(t, f.request("GET", "/auth/me", "", access), 401, "UNAUTHORIZED")
			if got := f.request("GET", "/auth/me", "", second.Result().Cookies()[0]); got.Code != 200 {
				t.Fatal("logout revoked another teacher session")
			}
			requireCode(t, f.request("POST", "/auth/teacher", `{}`), 422, "VALIDATION_ERROR")
			requireCode(t, f.request("POST", "/auth/register", `{"email":"shared-teacher@ai-tutor.invalid","password":"`+password+`"}`), 422, "VALIDATION_ERROR")
		})
	}
}

func TestConcurrentTeacherLoginUsesOneAccount(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := f.request("POST", "/auth/teacher", ""); w.Code != 200 {
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
