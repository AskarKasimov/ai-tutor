package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareDoesNotInterpretOrigin(t *testing.T) {
	a := &App{cfg: DefaultConfig()}
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"} {
		for _, source := range []string{"", "null", "https://frontend.example", "https://another.example"} {
			t.Run(method+"/"+source, func(t *testing.T) {
				called := false
				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				})
				req := httptest.NewRequest(method, "/auth/logout", nil)
				if source != "" {
					req.Header.Set("Origin", source)
				}
				if method == "OPTIONS" {
					req.Header.Set("Access-Control-Request-Method", "POST")
					req.Header.Set("Access-Control-Request-Headers", "content-type")
				}
				w := httptest.NewRecorder()
				a.middleware(next).ServeHTTP(w, req)
				if !called || w.Code != http.StatusNoContent {
					t.Fatalf("request did not reach handler: called=%t status=%d", called, w.Code)
				}
				for header := range w.Header() {
					if strings.HasPrefix(header, "Access-Control-") {
						t.Errorf("unexpected CORS header: %s", header)
					}
				}
				if w.Header().Get("Vary") != "" {
					t.Error("unexpected origin-dependent response")
				}
			})
		}
	}
}
