package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"go.uber.org/zap"
)

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// StatusClientClosedRequest is logged when the client went away before the
// response (nginx convention). It is not a server failure, so it is not
// logged as an error and never reaches the client.
const StatusClientClosedRequest = 499

func Error(ctx context.Context, w http.ResponseWriter, err error) {
	if errors.Is(ctx.Err(), context.Canceled) {
		// The browser aborted the request, for example a voice request when
		// the student starts recording: not an unavailable provider.
		recordFailure(ctx, "CLIENT_CLOSED_REQUEST", err)
		w.WriteHeader(StatusClientClosedRequest)
		return
	}
	var e *fault.Error
	// The cause behind a fault (a provider status, a model failure) is logged
	// with the request, while the client gets only the fault.
	internalErr := err
	if !errors.As(err, &e) {
		e = fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Обработчик временно недоступен.")
	} else if err == error(e) {
		internalErr = nil
	}
	if !recordFailure(ctx, e.Code, internalErr) && internalErr != nil {
		LoggerFromContext(ctx).Error("request failed", zap.Error(internalErr))
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
