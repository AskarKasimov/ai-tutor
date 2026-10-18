package modelapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSynthesizeStreamFlushAndOldProviderFallback(t *testing.T) {
	for _, status := range []int{200, 404, 406, 501, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			release := make(chan struct{})
			first := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/prefix/synthesize" {
					w.Write(testWAV())
					return
				}
				if r.URL.Path != "/prefix/synthesize/stream" {
					t.Errorf("path %s", r.URL.Path)
				}
				if status != 200 {
					w.WriteHeader(status)
					return
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["text"] != "Привет!!!" {
					t.Errorf("text %v", body)
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				fmt.Fprintln(w, `{"type":"start","version":1,"sample_rate":24000,"channels":1,"encoding":"pcm_s16le"}`)
				fmt.Fprintln(w, `{"type":"audio","seq":0,"data":"AQACAA=="}`)
				w.(http.Flusher).Flush()
				<-release
				fmt.Fprintln(w, `{"type":"end","chunks":1,"samples":2}`)
			}))
			defer server.Close()
			c := New(server.Client(), "", server.URL+"/prefix/synthesize", time.Second, TTSOptions{})
			done := make(chan error, 1)
			go func() {
				done <- c.SynthesizeStream(context.Background(), "Привет", func(e audio.StreamEvent) error {
					if e.Type == "audio" && status == 200 {
						close(first)
					}
					return nil
				})
			}()
			if status == 200 {
				select {
				case <-first:
				case <-time.After(time.Second):
					t.Fatal("no incremental audio")
				}
				close(release)
			}
			err := <-done
			if status == 503 {
				if err == nil || calls != 1 {
					t.Fatalf("5xx retried %d %v", calls, err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if status != 200 && calls != 2 {
				t.Fatalf("fallback calls %d", calls)
			}
		})
	}
}

func TestStreamDisconnectCancelsProviderWithoutWAVRetry(t *testing.T) {
	cancelled := make(chan struct{})
	calls := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"type":"start","version":1,"sample_rate":24000,"channels":1,"encoding":"pcm_s16le"}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	c := New(server.Client(), "", server.URL+"/synthesize", time.Second*3, TTSOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	err := c.SynthesizeStream(ctx, "Привет", func(e audio.StreamEvent) error { cancel(); return ctx.Err() })
	if err == nil {
		t.Fatal("cancelled stream succeeded")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("provider not cancelled")
	}
	if calls.Load() != 1 {
		t.Fatal("stream retried")
	}
}
