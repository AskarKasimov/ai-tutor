package postgres

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireSubject verifies the common course exists. Direction-specific data
// ownership is enforced by each module's resource queries, not a course mode.
func RequireSubject(pool *pgxpool.Pool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var allowed bool
		if err := pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM subjects WHERE id=$1)`, r.PathValue("subject_id")).Scan(&allowed); err != nil {
			httpx.Error(r.Context(), w, err)
			return
		}
		if !allowed {
			httpx.Error(r.Context(), w, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Предмет не найден."))
			return
		}
		next.ServeHTTP(w, r)
	})
}
