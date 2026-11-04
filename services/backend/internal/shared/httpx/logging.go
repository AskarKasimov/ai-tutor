package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/logx"
	"go.uber.org/zap"
)

type requestStateKey struct{}

type requestState struct {
	mu      sync.Mutex
	code    string
	message string
	err     error
}

func (s *requestState) record(code, message string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code, s.message, s.err = code, message, err
}

func (s *requestState) snapshot() (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code, s.message, s.err
}

func recordFailure(ctx context.Context, code, message string, err error) bool {
	state, ok := ctx.Value(requestStateKey{}).(*requestState)
	if ok {
		state.record(code, message, err)
	}
	return ok
}

// PropagateTrace makes an internal service continue the trace of the request.
func PropagateTrace(req *http.Request) {
	if traceparent := logx.Traceparent(req.Context()); traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
}

func RequestLogger(logger *zap.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := headerID(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = randomID()
		}
		traceID := traceID(r)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Trace-ID", traceID)

		requestLogger := logger.With(zap.String("request_id", requestID), zap.String("trace_id", traceID))
		state := &requestState{}
		ctx := logx.WithTrace(logx.With(context.WithValue(r.Context(), requestStateKey{}, state), requestLogger), traceID)
		r = r.WithContext(ctx)
		started := time.Now()
		response := &statusWriter{ResponseWriter: w}
		defer func() {
			panicked := recover()
			if response.status == 0 {
				response.status = http.StatusOK
			}
			if panicked != nil && response.status < http.StatusInternalServerError {
				response.status = http.StatusInternalServerError
			}
			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", response.status),
				zap.Duration("duration", time.Since(started)),
			}
			if code, message, err := state.snapshot(); code != "" {
				fields = append(fields, zap.String("error_code", code))
				if message != "" {
					fields = append(fields, zap.String("error_message", message))
				}
				if err != nil {
					fields = append(fields, zap.Error(err))
				}
			}
			if panicked != nil {
				fields = append(fields, zap.Bool("panicked", true), zap.Any("panic", panicked), zap.Stack("stacktrace"))
			}
			if response.status >= http.StatusInternalServerError || panicked != nil {
				requestLogger.Error("http request", fields...)
			} else if r.URL.Path == "/health" {
				// Container probes run every few seconds and bury the real requests.
				requestLogger.Debug("http request", fields...)
			} else {
				requestLogger.Info("http request", fields...)
			}
			if panicked != nil {
				panic(panicked)
			}
		}()
		next.ServeHTTP(response, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func traceID(r *http.Request) string {
	if traceID := traceParentID(r.Header.Get("traceparent")); traceID != "" {
		return traceID
	}
	if traceID := r.Header.Get("X-Trace-ID"); validHex(traceID, 32) && !allZero(traceID) {
		return strings.ToLower(traceID)
	}
	return randomID()
}

func traceParentID(value string) string {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || len(parts[0]) != 2 || parts[0] == "ff" ||
		!validHex(parts[1], 32) || allZero(parts[1]) ||
		!validHex(parts[2], 16) || allZero(parts[2]) || !validHex(parts[3], 2) {
		return ""
	}
	return strings.ToLower(parts[1])
}

func headerID(value string) string {
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
			return ""
		}
	}
	return value
}

func validHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func allZero(value string) bool {
	for _, char := range value {
		if char != '0' {
			return false
		}
	}
	return true
}

func randomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value)
}
