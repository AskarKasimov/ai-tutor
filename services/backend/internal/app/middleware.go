package app

import (
	"net/http"
	"strings"

	graphhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/transport/http"
)

func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		limit := int64(32 * 1024)
		if strings.HasPrefix(r.URL.Path, "/exam/admin/subjects/") && strings.HasSuffix(r.URL.Path, "/materials") {
			limit = 2500 * 1024 * 1024
		} else if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/exam/admin/subjects/") && strings.HasSuffix(r.URL.Path, "/raw") {
			limit = 51 * 1024 * 1024
		} else if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/exam/admin/subjects/") && strings.HasSuffix(r.URL.Path, "/graph/competency-map") {
			// The handler bounds the book itself and answers 413 above it.
			limit = graphhttp.MaxMapBytes + 2<<20
		} else if r.URL.Path == "/voice/transcriptions" {
			limit = a.cfg.MaxUploadBytes + 64*1024
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
