package gateway

import (
	"context"
	"errors"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/execution"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/testutil"
	"io"
	"log/slog"
	"testing"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/voicegateway/v1"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeProvider struct{ stt, tts string }

func (p *fakeProvider) Transcribe(_ context.Context, instance string, data []byte, media string) (string, error) {
	p.stt = instance
	return "текст", nil
}
func (p *fakeProvider) Synthesize(_ context.Context, instance, profile, text string) ([]byte, error) {
	p.tts = instance
	return testutil.WAV(), nil
}

func TestDispatchAndValidation(t *testing.T) {
	stt, err := scheduler.New([]scheduler.Instance{{ID: "stt", Slots: 1}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	tts, err := scheduler.New([]scheduler.Instance{{ID: "tts", Slots: 1, Profiles: []string{"ru"}}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{}
	blobs, err := journal.OpenBlobs(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer blobs.Close()
	engine := execution.New(&testutil.Memory{}, blobs, map[string]*scheduler.Queue{"stt": stt, "tts": tts}, time.Hour, func(string) bool { return false }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err = engine.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := New(engine, p, map[string]string{"ru": "rev"}, time.Second)
	response, err := s.Transcribe(context.Background(), &pb.TranscribeRequest{Audio: testutil.WAV(), MediaType: "audio/wav"})
	if err != nil || response.Text != "текст" || p.stt != "stt" {
		t.Fatalf("stt: %+v %v", response, err)
	}
	result, err := s.Synthesize(context.Background(), &pb.SynthesizeRequest{Text: "Привет", VoiceProfile: "ru", VoiceRevision: "rev"})
	if err != nil || len(result.Audio) != 46 || p.tts != "tts" {
		t.Fatalf("tts: %+v %v", result, err)
	}
	_, err = s.Synthesize(context.Background(), &pb.SynthesizeRequest{Text: "Привет", VoiceProfile: "other"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unavailable: %v", err)
	}
	_, err = s.Transcribe(context.Background(), &pb.TranscribeRequest{Audio: []byte("audio"), MediaType: "image/png"})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("invalid: %v", err)
	}
	if status.Code(mapError(errors.New("private upstream error"))) != codes.Unavailable {
		t.Fatal("unexpected status")
	}
}
