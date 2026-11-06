package execution

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockedMetricsWriter struct {
	*httptest.ResponseRecorder
	started chan struct{}
	release chan struct{}
}

func (w *blockedMetricsWriter) Write(data []byte) (int, error) {
	select {
	case <-w.started:
	default:
		close(w.started)
	}
	<-w.release
	return w.ResponseRecorder.Write(data)
}

func TestSlowMetricsScrapeDoesNotBlockRequests(t *testing.T) {
	e := setup(t, &testutil.Memory{}, t.TempDir())
	e.RecordRequest("stt", nil)
	w := &blockedMetricsWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
	scraped := make(chan struct{})
	go func() {
		defer close(scraped)
		e.Metrics(w, httptest.NewRequest("GET", "/metrics", nil))
	}()
	t.Cleanup(func() { close(w.release); <-scraped })
	select {
	case <-w.started:
	case <-time.After(5 * time.Second):
		t.Fatal("scrape did not start writing")
	}
	finished := make(chan error, 1)
	go func() {
		e.RecordRequest("stt", nil)
		e.RecordRequest("tts", status.Error(codes.ResourceExhausted, "busy"))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		r, _ := e.Submit(ctx, "stt", "", "", []byte("request"), func(string) {}, func(context.Context, string) scheduler.Result {
			return scheduler.Result{Value: []byte("result")}
		})
		finished <- r.Err
	}()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("slow scrape blocked request recording or execution")
	}
	// A second scrape sees the new counters even while the first is blocked.
	current := httptest.NewRecorder()
	e.Metrics(current, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(current.Body.String(), `voice_requests_total{operation="stt",code="OK",reason="ok"} 2`) {
		t.Fatalf("request counters not updated: %s", current.Body.String())
	}
	if !strings.Contains(current.Body.String(), `voice_requests_total{operation="tts",code="ResourceExhausted",reason="resourceexhausted"} 1`) {
		t.Fatalf("refusal counter not updated: %s", current.Body.String())
	}
}
