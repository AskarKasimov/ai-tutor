package metrics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestVoiceMetricsHaveBoundedLabelsAndHistograms(t *testing.T) {
	v := NewVoice()
	v.Provider(WAV, Success, time.Second)
	v.Provider(Stream, Error, 2*time.Second)
	v.Fallback()
	v.Cache(CacheHit)
	v.Cache(CacheWriteError)
	v.Stream(Leader, Success, 100*time.Millisecond, time.Second)
	v.Stream(Joined, Cancelled, 0, time.Second)
	var b bytes.Buffer
	if err := v.WritePrometheus(&b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`backend_tts_provider_requests_total{mode="wav",outcome="success"} 1`, `backend_tts_provider_requests_total{mode="stream",outcome="error"} 1`, `backend_tts_provider_fallback_total 1`, `backend_tts_cache_total{result="hit"} 1`, `backend_tts_cache_total{result="write_error"} 1`, `backend_tts_stream_requests_total{source="leader",outcome="success"} 1`, `backend_tts_stream_requests_total{source="joined",outcome="cancelled"} 1`, `backend_tts_stream_ttfa_seconds_count{source="leader"} 1`, `backend_tts_stream_ttfa_seconds_count{source="joined"} 0`, `backend_tts_stream_ttfa_seconds_bucket{source="leader",le="+Inf"} 1`} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %s", want)
		}
	}
	before := b.String()
	v.Provider(ProviderMode(99), Outcome(99), 0)
	v.Cache(CacheResult(99))
	v.Stream(StreamSource(99), Outcome(99), 0, 0)
	b.Reset()
	_ = v.WritePrometheus(&b)
	if before != b.String() {
		t.Fatal("arbitrary label created series")
	}
}
