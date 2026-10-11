package authhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

func TestCookiesUseRootPathForIssuanceAndDeletion(t *testing.T) {
	for _, portal := range []string{"", "admin"} {
		want := map[string]bool{"access_token": true, "refresh_token": true}
		if portal == "admin" {
			want = map[string]bool{"admin_access_token": true, "admin_refresh_token": true}
		}
		for _, deleting := range []bool{false, true} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/", nil)
			if portal != "" {
				r.Header.Set(httpx.PortalHeader, portal)
			}
			if deleting {
				clearCookies(w, r)
			} else {
				setCookies(w, r, session.Tokens{Access: "access", Refresh: "refresh"}, 0)
			}
			cookies := w.Result().Cookies()
			if len(cookies) != 2 {
				t.Fatalf("expected both session cookies, got %d", len(cookies))
			}
			for _, cookie := range cookies {
				if !want[cookie.Name] {
					t.Errorf("portal=%q issued %s", portal, cookie.Name)
				}
				if cookie.Path != "/" {
					t.Errorf("deleting=%t %s has Path=%q, want /", deleting, cookie.Name, cookie.Path)
				}
				if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
					t.Errorf("cookie attributes changed: %s", cookie)
				}
				if deleting && (cookie.Value != "" || cookie.MaxAge != -1) {
					t.Errorf("cookie is not deleted: %s", cookie)
				}
			}
		}
	}
}
