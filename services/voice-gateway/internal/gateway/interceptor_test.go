package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/voicegateway/v1"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/execution"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/provider"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func observedEngine(t *testing.T, quota int64, logger *slog.Logger) (*execution.Engine, *scheduler.Queue, *scheduler.Queue) {
	t.Helper()
	blobs, err := journal.OpenBlobs(t.TempDir(), quota)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(blobs.Close)
	stt, _ := scheduler.New([]scheduler.Instance{{ID: "stt", Slots: 1}}, 1)
	tts, _ := scheduler.New([]scheduler.Instance{{ID: "tts", Slots: 1, Profiles: []string{"voice"}}}, 1)
	e := execution.New(&testutil.Memory{}, blobs, map[string]*scheduler.Queue{"stt": stt, "tts": tts}, time.Hour, func(string) bool { return false }, logger)
	if err := e.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Wait)
	return e, stt, tts
}
func metrics(e *execution.Engine) string {
	w := httptest.NewRecorder()
	e.Metrics(w, httptest.NewRequest("GET", "/metrics", nil))
	return w.Body.String()
}
func TestAdmissionIsIndependentAndCountsRefusals(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, _, _ := observedEngine(t, 1<<20, logger)
	intercept := Interceptor(2, e, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	var wg sync.WaitGroup
	handler := func(ctx context.Context, _ any) (any, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, nil
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = intercept(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/voicegateway.v1.VoiceGateway/Synthesize"}, handler)
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("TTS did not fill admission")
		}
	}
	_, err := intercept(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/voicegateway.v1.VoiceGateway/Synthesize"}, handler)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("admission: %v", err)
	}
	_, err = intercept(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/voicegateway.v1.VoiceGateway/Transcribe"}, func(context.Context, any) (any, error) { return nil, nil })
	if err != nil {
		t.Fatalf("STT blocked by TTS: %v", err)
	}
	if !strings.Contains(metrics(e), `voice_requests_total{operation="tts",code="ResourceExhausted",reason="rpc_limit"} 1`) {
		t.Fatal(metrics(e))
	}
	// Cancel the held handlers and ensure they leave before closing resources.
	cancel()
	wg.Wait()
}
func TestLogsLinkRequestAndModelAndPreserveSafeReasons(t *testing.T) {
	for _, tc := range []struct {
		name       string
		quota      int64
		httpStatus int
		body       string
		want       string
	}{
		{"dial", 1 << 20, 0, "", "connection_failed"},
		{"http", 1 << 20, 503, "private model details", "http_response"},
		{"quota", 1, 200, `{"text":"private response"}`, "payload_quota"},
		{"result_write", 100, 200, `{"text":"` + strings.Repeat("x", 1024) + `"}`, "result_write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			e, _, _ := observedEngine(t, tc.quota, logger)
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.httpStatus)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer model.Close()
			if tc.name == "dial" {
				model.Close()
			}
			p := &provider.HTTP{STT: map[string]provider.STT{"stt": {URL: model.URL, Timeout: time.Second}}}
			server := New(e, p, map[string]string{"voice": "rev"}, time.Second)
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"))
			_, err := Interceptor(4, e, logger)(ctx, &pb.TranscribeRequest{Audio: testutil.WAV(), MediaType: "audio/wav"}, &grpc.UnaryServerInfo{FullMethod: "/voicegateway.v1.VoiceGateway/Transcribe"}, func(ctx context.Context, req any) (any, error) {
				return server.Transcribe(ctx, req.(*pb.TranscribeRequest))
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			entries := strings.Split(strings.TrimSpace(logs.String()), "\n")
			id := ""
			for _, line := range entries {
				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				if entry["error"] != tc.want || entry["error_code"] == nil || entry["stage"] == nil {
					t.Fatalf("missing safe diagnostic: %s", line)
				}
				if entry["trace_id"] != "0123456789abcdef0123456789abcdef" {
					t.Fatal(line)
				}
				got, _ := entry["gateway_request_id"].(string)
				if len(got) != 32 || (id != "" && id != got) {
					t.Fatal(line)
				}
				id = got
				if tc.httpStatus == 503 && entry["upstream_http_status"] != float64(503) {
					t.Fatal(line)
				}
			}
			if tc.name != "quota" && len(entries) != 2 {
				t.Fatalf("request/upstream events: %s", logs.String())
			}
			if strings.Contains(logs.String(), model.URL) || strings.Contains(logs.String(), "private") {
				t.Fatal("private content leaked")
			}
			if !strings.Contains(metrics(e), `reason="`+tc.want+`"} 1`) {
				t.Fatal(metrics(e))
			}
		})
	}
}
func TestTruncatedResponseRetainsModelSlot(t *testing.T) {
	for _, op := range []string{"stt", "tts"} {
		t.Run(op, func(t *testing.T) {
			e, stt, tts := observedEngine(t, 1<<20, slog.New(slog.NewTextHandler(io.Discard, nil)))
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "1024")
				_, _ = w.Write([]byte("x"))
			}))
			defer model.Close()
			p := &provider.HTTP{STT: map[string]provider.STT{"stt": {URL: model.URL, Timeout: time.Second}}, TTS: map[string]provider.TTS{"tts": {URL: model.URL, Timeout: time.Second, Profiles: map[string]provider.Options{"voice": {}}}}}
			s := New(e, p, map[string]string{"voice": "rev"}, time.Second)
			var err error
			if op == "stt" {
				_, err = s.Transcribe(context.Background(), &pb.TranscribeRequest{Audio: testutil.WAV(), MediaType: "audio/wav"})
			} else {
				_, err = s.Synthesize(context.Background(), &pb.SynthesizeRequest{Text: "Привет", VoiceProfile: "voice", VoiceRevision: "rev"})
			}
			if err == nil {
				t.Fatal("truncated body accepted")
			}
			q := stt
			if op == "tts" {
				q = tts
			}
			if q.Stats().Instances[0].Busy != 1 {
				t.Fatal("model slot freed after truncated response")
			}
		})
	}
}

func TestQueueRefusalIsVisibleWithoutModelCall(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, stt, _ := observedEngine(t, 1<<20, logger)
	if err := stt.RestoreHeld("stt", 1); err != nil {
		t.Fatal(err)
	}
	intercept := Interceptor(8, e, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	handler := func(ctx context.Context, _ any) (any, error) {
		r, _ := e.Submit(ctx, "stt", "", "", []byte("audio"), func(string) {}, func(context.Context, string) scheduler.Result { return scheduler.Result{Err: io.EOF} })
		return nil, mapError(r.Err)
	}
	go func() {
		_, _ = intercept(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/voicegateway.v1.VoiceGateway/Transcribe"}, handler)
		close(done)
	}()
	deadline := time.Now().Add(time.Second)
	for stt.Stats().Waiting != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stt.Stats().Waiting != 1 {
		t.Fatal("request did not queue")
	}
	_, err := intercept(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/voicegateway.v1.VoiceGateway/Transcribe"}, handler)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("queue refusal: %v", err)
	}
	snapshot := metrics(e)
	if !strings.Contains(snapshot, `voice_requests_total{operation="stt",code="ResourceExhausted",reason="queue_full"} 1`) {
		t.Fatal(snapshot)
	}
	if strings.Contains(snapshot, `voice_upstream_calls_total{`) {
		t.Fatal("queued failure counted as model call")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queue cancellation stuck")
	}
}
