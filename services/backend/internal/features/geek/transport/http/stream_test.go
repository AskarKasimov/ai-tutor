package geek

import (
	"context"
	"encoding/json"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamFrameFlushesAndDoesNotBuffer(t *testing.T) {
	w := httptest.NewRecorder()
	s := newStreamResponse(context.Background(), w)
	if err := s.emit(audio.StartEvent(24000)); err != nil {
		t.Fatal(err)
	}
	if !w.Flushed || w.Header().Get("Content-Type") != "application/x-ndjson" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Length") != "" {
		t.Fatalf("headers %v flushed %v", w.Header(), w.Flushed)
	}
	if err := s.emit(audio.StreamEvent{Type: "audio", Seq: 0, PCM: []byte{1, 0}}); err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(w.Body)
	var e map[string]any
	_ = d.Decode(&e)
	_ = d.Decode(&e)
	if e["seq"] != float64(0) || e["data"] != "AQA=" {
		t.Fatalf("audio %v", e)
	}
}
func TestTurnStreamRequiresPrincipalBeforeService(t *testing.T) {
	w := httptest.NewRecorder()
	New(nil).TurnAudioStream(w, httptest.NewRequest(http.MethodGet, "/diagnostic-sessions/a/turns/b/audio/stream", nil))
	if w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
}

type deadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineWriter) SetWriteDeadline(d time.Time) error { w.deadline = d; return nil }
func TestStreamWriteDeadlineDoesNotLimitGenerationPause(t *testing.T) {
	w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	s := newStreamResponse(context.Background(), w)
	if err := s.emit(audio.StartEvent(24000)); err != nil {
		t.Fatal(err)
	}
	if !w.deadline.IsZero() {
		t.Fatal("write deadline remains active during model generation")
	}
}
