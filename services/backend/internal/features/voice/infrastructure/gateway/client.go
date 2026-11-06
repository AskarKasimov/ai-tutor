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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
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
}

func NewSynthesizer(client pb.VoiceGatewayClient, timeout time.Duration, profile, revision string) *Synthesizer {
	return &Synthesizer{client: client, timeout: timeout, profile: profile, revision: revision}
}

func (s *Synthesizer) Synthesize(ctx context.Context, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	response, err := s.client.Synthesize(ctx, &pb.SynthesizeRequest{Text: text, VoiceProfile: s.profile, VoiceRevision: s.revision})
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

// Dial resolves the voice revision once at startup. A changed revision is
// rejected by synthesis until backend refreshes its cache identity on restart.
func Dial(ctx context.Context, address, profile string) (pb.VoiceGatewayClient, *grpc.ClientConn, string, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(interceptor), grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(26<<20), grpc.MaxCallRecvMsgSize(65<<20)))
	if err != nil {
		return nil, nil, "", fmt.Errorf("configure voice gateway: %w", err)
	}
	client := pb.NewVoiceGatewayClient(conn)
	description, err := client.DescribeVoice(ctx, &pb.DescribeVoiceRequest{VoiceProfile: profile})
	if err != nil || description.GetRevision() == "" {
		_ = conn.Close()
		return nil, nil, "", fmt.Errorf("resolve voice profile: %s", status.Code(err))
	}
	return client, conn, description.Revision, nil
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
