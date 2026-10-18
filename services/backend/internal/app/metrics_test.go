package app

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/metrics"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandlerExportsVoiceMeasurements(t *testing.T) {
	a := &App{voiceMetrics: metrics.NewVoice()}
	a.voiceMetrics.Stream(metrics.Joined, metrics.Success, 200*time.Millisecond, time.Second)
	w := httptest.NewRecorder()
	a.metrics(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" || !strings.Contains(w.Body.String(), `backend_tts_stream_requests_total{source="joined",outcome="success"} 1`) {
		t.Fatalf("metrics %d %v %s", w.Code, w.Header(), w.Body.String())
	}
}
