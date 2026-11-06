// Package gateway adapts the voice gateway contract to the application's ports.
package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/logx"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/voicegateway/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Recognizer struct {
	client  pb.VoiceGatewayClient
	timeout time.Duration
}

func NewRecognizer(client pb.VoiceGatewayClient, timeout time.Duration) *Recognizer {
	return &Recognizer{client: client, timeout: timeout}
}

func (r *Recognizer) Recognize(ctx context.Context, data []byte, media string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	response, err := r.client.Transcribe(ctx, &pb.TranscribeRequest{Audio: data, MediaType: media})
	if err != nil {
		return "", gatewayError(err, true)
	}
	if response == nil || !utf8.ValidString(response.Text) || strings.ContainsRune(response.Text, 0) {
		return "", fault.New(fault.Upstream, "TRANSCRIPTION_FAILED", "Некорректный ответ сервиса распознавания.")
	}
	return strings.TrimSpace(response.Text), nil
}

type Synthesizer struct {
	client   pb.VoiceGatewayClient
	timeout  time.Duration
	profile  string
	revision string
	mu       sync.Mutex
	lookup   singleflight.Group
}

func NewSynthesizer(client pb.VoiceGatewayClient, timeout time.Duration, profile string) *Synthesizer {
	return &Synthesizer{client: client, timeout: timeout, profile: profile}
}

// VoiceName resolves the cache identity lazily and retains it for this process.
func (s *Synthesizer) VoiceName(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", gatewayError(err, false)
	}
	s.mu.Lock()
	revision := s.revision
	s.mu.Unlock()
	if revision != "" {
		return s.profile + ":" + revision, nil
	}
	result := s.lookup.DoChan("revision", func() (any, error) {
		// A previous lookup can complete between the cache read and DoChan.
		s.mu.Lock()
		revision := s.revision
		s.mu.Unlock()
		if revision != "" {
			return s.profile + ":" + revision, nil
		}
		// One caller leaving must not cancel the lookup for other waiters.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()
		description, err := s.client.DescribeVoice(ctx, &pb.DescribeVoiceRequest{VoiceProfile: s.profile})
		if err != nil {
			return "", gatewayError(err, false)
		}
		if description.GetRevision() == "" {
			return "", fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Голосовой сервис временно недоступен.")
		}
		s.mu.Lock()
		s.revision = description.Revision
		s.mu.Unlock()
		return s.profile + ":" + description.Revision, nil
	})
	select {
	case <-ctx.Done():
		return "", gatewayError(ctx.Err(), false)
	case r := <-result:
		if r.Err != nil {
			return "", r.Err
		}
		return r.Val.(string), nil
	}
}

func (s *Synthesizer) Synthesize(ctx context.Context, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	name, err := s.VoiceName(ctx)
	if err != nil {
		return nil, err
	}
	revision := strings.TrimPrefix(name, s.profile+":")
	response, err := s.client.Synthesize(ctx, &pb.SynthesizeRequest{Text: text, VoiceProfile: s.profile, VoiceRevision: revision})
	if err != nil {
		return nil, gatewayError(err, false)
	}
	if response == nil || response.MediaType != "audio/wav" || !audio.ValidWAV(response.Audio) {
		return nil, fault.New(fault.Upstream, "SYNTHESIS_FAILED", "Сервис синтеза не вернул корректный WAV.")
	}
	return response.Audio, nil
}

func gatewayError(err error, stt bool) error {
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fault.New(fault.Timeout, "PROCESSING_TIMEOUT", "Превышено время ожидания голосового сервиса.")
	}
	switch status.Code(err) {
	case codes.DeadlineExceeded:
		return fault.New(fault.Timeout, "PROCESSING_TIMEOUT", "Превышено время ожидания голосового сервиса.")
	case codes.InvalidArgument:
		if stt {
			return fault.New(fault.Invalid, "INVALID_AUDIO", "Некорректные данные для голосового сервиса.")
		}
		return fault.New(fault.Invalid, "INVALID_TEXT", "Некорректный текст для озвучивания.")
	case codes.ResourceExhausted, codes.Unavailable, codes.FailedPrecondition, codes.NotFound, codes.Aborted:
		return fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Голосовой сервис временно недоступен.")
	case codes.OutOfRange:
		return fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Файл превышает 25 МиБ.")
	case codes.Unimplemented:
		return fault.New(fault.Unsupported, "UNSUPPORTED_AUDIO_FORMAT", "Неподдерживаемый формат записи.")
	default:
		code := "SYNTHESIS_FAILED"
		if stt {
			code = "TRANSCRIPTION_FAILED"
		}
		return fault.New(fault.Upstream, code, "Голосовой сервис не вернул корректный результат.")
	}
}

// Dial creates a connection without requiring the gateway to be available at startup.
func Dial(address string) (pb.VoiceGatewayClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(interceptor), grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(26<<20), grpc.MaxCallRecvMsgSize(65<<20)))
	if err != nil {
		return nil, nil, fmt.Errorf("configure voice gateway: %w", err)
	}
	return pb.NewVoiceGatewayClient(conn), conn, nil
}
func interceptor(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if traceparent := logx.Traceparent(ctx); traceparent != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", traceparent)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "idempotency-key", hex.EncodeToString(id))
	var header metadata.MD
	started := time.Now()
	err := invoke(ctx, method, req, reply, conn, append(opts, grpc.Header(&header))...)
	fields := []zap.Field{zap.String("voice_operation", method), zap.Duration("duration", time.Since(started)), zap.String("grpc_code", status.Code(err).String())}
	if values := header.Get("x-request-id"); len(values) == 1 {
		fields = append(fields, zap.String("gateway_request_id", values[0]))
	}
	if err == nil || status.Code(err) == codes.Canceled {
		logx.From(ctx).Info("voice gateway call", fields...)
	} else {
		logx.From(ctx).Warn("voice gateway call failed", fields...)
	}
	return err
}
