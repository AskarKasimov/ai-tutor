package postgres

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireMode prevents an administrator from using a subject ID through the
// other mode's API namespace. Query-level mode predicates remain the data-layer
// defense; this middleware gives handlers a consistent not-found response.
func RequireMode(pool *pgxpool.Pool, mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var allowed bool
		if err := pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM subjects WHERE id=$1 AND mode=$2)`, r.PathValue("subject_id"), mode).Scan(&allowed); err != nil {
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
