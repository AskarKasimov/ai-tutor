// Package logx carries the logger and the trace of the current work in a
// context. It has no HTTP dependency, so application code may use it too.
package logx

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"go.uber.org/zap"
)

type loggerKey struct{}
type traceKey struct{}

// With returns a context whose logs go to logger.
func With(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// From returns the logger of ctx, or the global one for work started outside a
// request or a worker, so that a failure there is still written.
func From(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok && logger != nil {
		return logger
	}
	return zap.L()
}

// WithTrace records the trace that outgoing calls continue.
func WithTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceID)
}

// TraceID returns the trace of ctx, or "" outside a trace.
func TraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceKey{}).(string)
	return traceID
}

// StartTrace begins a trace for one unit of background work, such as a job a
// worker has picked up: its logs and its calls to other services share the ID.
func StartTrace(ctx context.Context) context.Context {
	traceID := randomHex(16)
	return With(WithTrace(ctx, traceID), From(ctx).With(zap.String("trace_id", traceID)))
}

// Traceparent returns the W3C header value that continues the trace of ctx, or
// "" when there is no trace to continue.
func Traceparent(ctx context.Context) string {
	traceID := TraceID(ctx)
	if traceID == "" {
		return ""
	}
	return "00-" + traceID + "-" + randomHex(8) + "-01"
}

func randomHex(size int) string {
	value := make([]byte, size)
	// crypto/rand does not fail on supported platforms.
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}
