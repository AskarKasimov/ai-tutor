package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type memoryRepository struct {
	saved []transcription.Transcription
	err   error
}

func (r *memoryRepository) Save(_ context.Context, tr transcription.Transcription) error {
	r.saved = append(r.saved, tr)
	return r.err
}

type recognizerFunc func(context.Context, []byte, string) (string, error)

func (f recognizerFunc) Recognize(ctx context.Context, data []byte, media string) (string, error) {
	return f(ctx, data, media)
}

type synthesizerFunc func(context.Context, string) ([]byte, error)

func (f synthesizerFunc) Synthesize(ctx context.Context, text string) ([]byte, error) {
	return f(ctx, text)
}

func opus() []byte { return append(append([]byte("OggS"), make([]byte, 23)...), []byte("OpusHead")...) }

func TestTranscribePersistsOwnerAndNormalizedText(t *testing.T) {
	repo := &memoryRepository{}
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	called := false
	provider := recognizerFunc(func(got context.Context, data []byte, media string) (string, error) {
		called = true
		if got != ctx || string(data) != string(opus()) || media != "audio/ogg; codecs=opus" {
			t.Fatal("recognizer arguments changed")
		}
		return "  Ответ ученика. \n", nil
	})
	s := New(repo, provider, nil, func() time.Time { return time.Unix(1790762400, 0) })
	tr, err := s.Transcribe(ctx, "student_1", opus(), "audio/ogg; codecs=opus")
	if err != nil {
		t.Fatal(err)
	}
	if !called || len(repo.saved) != 1 || repo.saved[0] != tr || tr.OwnerID != "student_1" || tr.Text != "Ответ ученика." || tr.CreatedAt != 1790762400 || !strings.HasPrefix(tr.ID, "transcription_") {
		t.Fatalf("wrong persisted transcription: %+v", tr)
	}
}

func TestFailedTranscriptionsNeverSave(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		data       []byte
		err        error
		code       string
	}{
		{name: "invalid upload", data: nil, code: "INVALID_AUDIO"},
		{name: "provider failed", data: opus(), err: fault.New(fault.Timeout, "PROCESSING_TIMEOUT", "timeout"), code: "PROCESSING_TIMEOUT"},
		{name: "no speech", data: opus(), text: " \n ", code: "NO_SPEECH_DETECTED"},
		{name: "nul", data: opus(), text: "bad\x00text", code: "TRANSCRIPTION_FAILED"},
		{name: "invalid utf8", data: opus(), text: string([]byte{0xff}), code: "TRANSCRIPTION_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &memoryRepository{}
			s := New(repo, recognizerFunc(func(context.Context, []byte, string) (string, error) { return tc.text, tc.err }), nil, nil)
			tr, err := s.Transcribe(context.Background(), "student_1", tc.data, "audio/ogg")
			var failure *fault.Error
			if !errors.As(err, &failure) || failure.Code != tc.code {
				t.Fatalf("wrong error: %v", err)
			}
			if len(repo.saved) != 0 || tr != (transcription.Transcription{}) {
				t.Fatal("failed result persisted or returned")
			}
		})
	}
}

func TestSynthesisValidatesUnicodeBeforeProvider(t *testing.T) {
	calls := 0
	s := New(nil, nil, synthesizerFunc(func(_ context.Context, text string) ([]byte, error) {
		calls++
		if text != strings.Repeat("я", 500) {
			t.Fatal("provider text changed")
		}
		return []byte("provider audio"), nil
	}), nil)
	for _, text := range []string{"", "   ", strings.Repeat("я", 501), "x\x00y"} {
		_, err := s.Synthesize(context.Background(), text)
		var failure *fault.Error
		if !errors.As(err, &failure) || failure.Code != "VALIDATION_ERROR" {
			t.Fatalf("invalid text accepted: %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("provider called for invalid text")
	}
	data, err := s.Synthesize(context.Background(), strings.Repeat("я", 500))
	if err != nil || string(data) != "provider audio" || calls != 1 {
		t.Fatalf("valid unicode rejected: %v", err)
	}
}
