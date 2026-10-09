package app

import (
	"net/http"
	"strings"
)

func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		limit := int64(32 * 1024)
		if r.URL.Path == "/admin/materials" {
			// 100,000 content runes may each occupy twelve bytes as JSON surrogate pairs.
			// Leave room for the name, outcome IDs and JSON structure.
			limit = 1280 * 1024
		}
		if r.URL.Path == "/voice/transcriptions" || r.URL.Path == "/admin/competency-map/import" ||
			(strings.HasPrefix(r.URL.Path, "/admin/subjects/") && strings.HasSuffix(r.URL.Path, "/competency-map/import")) ||
   ((strings.HasPrefix(r.URL.Path, "/diagnostic-sessions/") || strings.HasPrefix(r.URL.Path, "/training-sessions/")) && strings.HasSuffix(r.URL.Path, "/answers")) {
			limit = a.cfg.MaxUploadBytes + 64*1024
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
