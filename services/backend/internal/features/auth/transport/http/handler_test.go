package authhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/session"
)

func TestCookiesUseRootPathForIssuanceAndDeletion(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		w := httptest.NewRecorder()
		if deleting {
			clearCookies(w)
		} else {
			setCookies(w, session.Tokens{Access: "access", Refresh: "refresh"}, 0)
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 2 {
			t.Fatalf("expected both session cookies, got %d", len(cookies))
		}
		for _, cookie := range cookies {
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
