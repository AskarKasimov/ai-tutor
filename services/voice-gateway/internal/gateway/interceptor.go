package gateway

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/execution"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Interceptor keeps admission independent for STT, TTS and profile lookup.
func Interceptor(limit int, engine *execution.Engine, logger *slog.Logger) grpc.UnaryServerInterceptor {
	tokens := map[string]chan struct{}{}
	for _, op := range []string{"stt", "tts", "describe_voice", "other"} {
		tokens[op] = make(chan struct{}, limit)
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
		op := "other"
		switch strings.TrimPrefix(info.FullMethod, "/voicegateway.v1.VoiceGateway/") {
		case "Transcribe":
			op = "stt"
		case "Synthesize":
			op = "tts"
		case "DescribeVoice":
			op = "describe_voice"
		}
		ctx, err = diagnostic.Begin(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, "request identity unavailable")
		}
		started := time.Now()
		defer func() {
			engine.RecordRequest(op, err)
			attrs := []slog.Attr{slog.String("operation", op), slog.String("trace_id", diagnostic.TraceID(ctx)), slog.String("gateway_request_id", diagnostic.ID(ctx)), slog.Duration("duration", time.Since(started))}
			attrs = append(attrs, diagnostic.Fields(err)...)
			level := slog.LevelInfo
			if err != nil && status.Code(err) != codes.Canceled {
				level = slog.LevelWarn
			}
			logger.LogAttrs(ctx, level, "voice gateway request", attrs...)
		}()
		select {
		case tokens[op] <- struct{}{}:
			defer func() { <-tokens[op] }()
		default:
			return nil, diagnostic.New(codes.ResourceExhausted, "admission", "rpc_limit")
		}
		return handler(ctx, req)
	}
}
