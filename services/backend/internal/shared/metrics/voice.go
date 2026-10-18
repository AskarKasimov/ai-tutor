// Package metrics records operational measurements with fixed cardinality.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

type ProviderMode uint8

const (
	WAV ProviderMode = iota
	Stream
)

type Outcome uint8

const (
	Success Outcome = iota
	Error
	Cancelled
	Timeout
)

type StreamSource uint8

const (
	Leader StreamSource = iota
	Joined
)

type CacheResult uint8

const (
	CacheHit CacheResult = iota
	CacheMiss
	CacheBypass
	CacheReadError
	CacheWriteError
	CacheWritten
)

var providerNames = [2]string{"wav", "stream"}
var outcomeNames = [4]string{"success", "error", "cancelled", "timeout"}
var sourceNames = [2]string{"leader", "joined"}
var cacheNames = [6]string{"hit", "miss", "bypass", "read_error", "write_error", "written"}
var buckets = [13]float64{.05, .1, .2, .5, 1, 2, 5, 10, 30, 60, 120, 300, 600}

type histogram struct {
	Count   uint64
	Sum     float64
	Buckets [13]uint64
}

func (h *histogram) observe(d time.Duration) {
	seconds := d.Seconds()
	h.Count++
	h.Sum += seconds
	for i, b := range buckets {
		if seconds <= b {
			h.Buckets[i]++
		}
	}
}

type Voice struct {
	mu       sync.Mutex
	provider [2][4]histogram
	streams  [2][4]histogram
	ttfa     [2]histogram
	cache    [6]uint64
	fallback uint64
}

func NewVoice() *Voice { return &Voice{} }
func Result(err error) Outcome {
	switch {
	case err == nil:
		return Success
	case errors.Is(err, context.DeadlineExceeded):
		return Timeout
	case errors.Is(err, context.Canceled):
		return Cancelled
	default:
		return Error
	}
}
func (v *Voice) Provider(mode ProviderMode, outcome Outcome, d time.Duration) {
	if v == nil || int(mode) >= len(providerNames) || int(outcome) >= len(outcomeNames) {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.provider[mode][outcome].observe(d)
}
func (v *Voice) Stream(source StreamSource, outcome Outcome, ttfa, d time.Duration) {
	if v == nil || int(source) >= len(sourceNames) || int(outcome) >= len(outcomeNames) {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.streams[source][outcome].observe(d)
	if ttfa > 0 {
		v.ttfa[source].observe(ttfa)
	}
}
func (v *Voice) Cache(result CacheResult) {
	if v == nil || int(result) >= len(cacheNames) {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cache[result]++
}
func (v *Voice) Fallback() {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.fallback++
}

// WritePrometheus takes a coherent snapshot. No input-derived names or labels
// can enter this registry; memory remains fixed as traffic and texts grow.
func (v *Voice) WritePrometheus(w io.Writer) error {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	provider, streams, ttfa, cache, fallback := v.provider, v.streams, v.ttfa, v.cache, v.fallback
	v.mu.Unlock()
	var err error
	write := func(format string, args ...any) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, args...)
		}
	}
	for _, help := range [][2]string{
		{"backend_tts_provider_requests_total", "Logical provider syntheses by transport and outcome; fallback is counted once."},
		{"backend_tts_provider_duration_seconds", "Logical provider synthesis duration including fallback in seconds."},
		{"backend_tts_provider_fallback_total", "Unsupported streaming endpoints converted once to WAV."},
		{"backend_tts_cache_total", "Persistent WAV cache operations and bypasses."},
		{"backend_tts_stream_requests_total", "Authenticated PCM stream consumers by producer relationship and outcome."},
		{"backend_tts_stream_duration_seconds", "PCM stream consumer lifetime in seconds."},
		{"backend_tts_stream_ttfa_seconds", "Time to first successfully delivered PCM by producer relationship in seconds."},
	} {
		write("# HELP %s %s\n", help[0], help[1])
	}
	hist := func(name, labels string, h histogram) {
		for i, b := range buckets {
			write("%s_bucket{%s,le=\"%g\"} %d\n", name, labels, b, h.Buckets[i])
		}
		write("%s_bucket{%s,le=\"+Inf\"} %d\n", name, labels, h.Count)
		write("%s_count{%s} %d\n%s_sum{%s} %g\n", name, labels, h.Count, name, labels, h.Sum)
	}
	write("# TYPE backend_tts_provider_requests_total counter\n# TYPE backend_tts_provider_duration_seconds histogram\n")
	for m, mname := range providerNames {
		for o, oname := range outcomeNames {
			labels := fmt.Sprintf("mode=\"%s\",outcome=\"%s\"", mname, oname)
			write("backend_tts_provider_requests_total{%s} %d\n", labels, provider[m][o].Count)
			hist("backend_tts_provider_duration_seconds", labels, provider[m][o])
		}
	}
	write("# TYPE backend_tts_provider_fallback_total counter\nbackend_tts_provider_fallback_total %d\n# TYPE backend_tts_cache_total counter\n", fallback)
	for i, name := range cacheNames {
		write("backend_tts_cache_total{result=\"%s\"} %d\n", name, cache[i])
	}
	write("# TYPE backend_tts_stream_requests_total counter\n# TYPE backend_tts_stream_duration_seconds histogram\n# TYPE backend_tts_stream_ttfa_seconds histogram\n")
	for s, sname := range sourceNames {
		for o, oname := range outcomeNames {
			labels := fmt.Sprintf("source=\"%s\",outcome=\"%s\"", sname, oname)
			write("backend_tts_stream_requests_total{%s} %d\n", labels, streams[s][o].Count)
			hist("backend_tts_stream_duration_seconds", labels, streams[s][o])
		}
		hist("backend_tts_stream_ttfa_seconds", fmt.Sprintf("source=\"%s\"", sname), ttfa[s])
	}
	return err
}
