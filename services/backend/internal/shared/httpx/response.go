package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Error(w http.ResponseWriter, err error) {
	var e *fault.Error
	if !errors.As(err, &e) {
		slog.Error("request failed", "error", err)
		e = fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Обработчик временно недоступен.")
	}
	status := http.StatusInternalServerError
	switch e.Kind {
	case fault.Invalid:
		status = 422
	case fault.Unauthorized:
		status = 401
	case fault.Forbidden:
		status = 403
	case fault.Conflict:
		status = 409
	case fault.TooLarge:
		status = 413
	case fault.Unsupported:
		status = 415
	case fault.Unavailable:
		status = 503
	case fault.Upstream:
		status = 502
	case fault.Timeout:
		status = 504
	case fault.NotFound:
		status = 404
	}
	JSON(w, status, e)
}

type principalKey struct{}

func WithPrincipal[T any](r *http.Request, p T) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
}
func Principal[T any](r *http.Request) (T, bool) {
	p, ok := r.Context().Value(principalKey{}).(T)
	return p, ok
}
