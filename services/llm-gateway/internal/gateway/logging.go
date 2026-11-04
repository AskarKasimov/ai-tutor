package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// NewLogger writes JSON lines with the field names shared by the services:
// ts, level, msg, trace_id, gateway_request_id, error_code, error, duration.
// Message and answer texts are never logged here, at any level.
func NewLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return attr
			}
			switch attr.Key {
			case slog.TimeKey:
				attr.Key = "ts"
			case slog.LevelKey:
				attr.Value = slog.StringValue(strings.ToLower(attr.Value.String()))
			}
			return attr
		},
	}))
}

// ParseLogLevel reads the log_level setting.
func ParseLogLevel(value string) (slog.Level, error) {
	switch value {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("log_level must be debug, info, warn or error")
}

// traceIDFromParent returns the trace of a W3C traceparent value, or "" for a
// missing or malformed one: the trace only labels logs.
func traceIDFromParent(value string) string {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || len(parts[1]) != 32 || strings.Trim(parts[1], "0") == "" {
		return ""
	}
	if strings.Trim(strings.ToLower(parts[1]), "0123456789abcdef") != "" {
		return ""
	}
	return strings.ToLower(parts[1])
}

func traceAttrs(traceID string) []any {
	if traceID == "" {
		return nil
	}
	return []any{"trace_id", traceID}
}

func logGRPCCall(ctx context.Context, in *pb.CompleteRequest, id string, err error, started time.Time) {
	md, _ := metadata.FromIncomingContext(ctx)
	traceID := ""
	if values := md.Get("traceparent"); len(values) > 0 {
		traceID = traceIDFromParent(values[0])
	}
	code := status.Code(err)
	attrs := append(traceAttrs(traceID), "transport", "grpc", "gateway_request_id", id, "complexity", in.GetComplexity().String(),
		"urgent", in.GetUrgent(), "grpc_code", code.String(), "duration", time.Since(started).Seconds())
	if err != nil {
		attrs = append(attrs, "error", status.Convert(err).Message())
		for _, detail := range status.Convert(err).Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok {
				attrs = append(attrs, "error_code", info.GetReason())
			}
		}
	}
	slog.Log(ctx, grpcLogLevel(code), "gateway request", attrs...)
}

func grpcLogLevel(code codes.Code) slog.Level {
	switch code {
	case codes.OK, codes.Canceled:
		return slog.LevelInfo
	case codes.InvalidArgument, codes.AlreadyExists, codes.ResourceExhausted:
		return slog.LevelWarn
	}
	return slog.LevelError
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logHTTP logs completion requests; health and metrics probes stay at debug.
func logHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		level := slog.LevelDebug
		if r.URL.Path == "/v1/chat/completions" {
			level = slog.LevelInfo
			if recorder.status >= 500 {
				level = slog.LevelError
			} else if recorder.status >= 400 {
				level = slog.LevelWarn
			}
		}
		attrs := append(traceAttrs(traceIDFromParent(r.Header.Get("traceparent"))), "transport", "http", "method", r.Method, "path", r.URL.Path,
			"status", recorder.status, "gateway_request_id", w.Header().Get("X-Request-ID"), "duration", time.Since(started).Seconds())
		slog.Log(r.Context(), level, "gateway request", attrs...)
	})
}
