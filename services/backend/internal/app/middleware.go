package app

import "net/http"

func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		limit := int64(32 * 1024)
		if r.URL.Path == "/v1/voice/transcriptions" || r.URL.Path == "/v1/admin/competency-map/import" {
			limit = a.cfg.MaxUploadBytes + 64*1024
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
