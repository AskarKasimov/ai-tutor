package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/voicegateway/v1"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/execution"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/provider"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/testutil"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestRedpandaGRPCModelProtocolsAndRestart(t *testing.T) {
	brokers := os.Getenv("TEST_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("TEST_KAFKA_BROKERS is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	admin, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	prefix := fmt.Sprintf("voice-test-%d", time.Now().UnixNano())
	sttTopic, ttsTopic := prefix+"-stt", prefix+"-tts"
	request := kmsg.NewPtrCreateTopicsRequest()
	request.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: sttTopic, NumPartitions: 1, ReplicationFactor: 1}, {Topic: ttsTopic, NumPartitions: 1, ReplicationFactor: 1}}
	response, err := admin.Request(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range response.(*kmsg.CreateTopicsResponse).Topics {
		if topic.ErrorCode != 0 {
			t.Fatalf("create topic code %d", topic.ErrorCode)
		}
	}
	defer func() {
		req := kmsg.NewPtrDeleteTopicsRequest()
		req.Topics = []kmsg.DeleteTopicsRequestTopic{{Topic: &sttTopic}, {Topic: &ttsTopic}}
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		response, err := admin.Request(cleanup, req)
		if err != nil {
			t.Errorf("delete temporary voice topics: %v", err)
			return
		}
		for _, topic := range response.(*kmsg.DeleteTopicsResponse).Topics {
			if topic.ErrorCode != 0 {
				t.Errorf("delete temporary voice topic: code %d", topic.ErrorCode)
			}
		}
	}()
	var sttCalls, ttsCalls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stt":
			sttCalls.Add(1)
			if err := r.ParseMultipartForm(26 << 20); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			defer r.MultipartForm.RemoveAll()
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			defer file.Close()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"text":"Распознанный ответ"}`))
		case "/tts":
			ttsCalls.Add(1)
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(testutil.WAV())
		default:
			w.WriteHeader(404)
		}
	}))
	defer model.Close()
	store, err := journal.New(strings.Split(brokers, ","), sttTopic, ttsTopic)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	blobs, err := journal.OpenBlobs(dir, 128<<20)
	if err != nil {
		t.Fatal(err)
	}
	makeEngine := func(b *journal.Blobs) *execution.Engine {
		stt, _ := scheduler.New([]scheduler.Instance{{ID: "stt-1", Slots: 1}}, 8)
		tts, _ := scheduler.New([]scheduler.Instance{{ID: "tts-1", Slots: 1, Profiles: []string{"voice"}}}, 8)
		e := execution.New(store, b, map[string]*scheduler.Queue{"stt": stt, "tts": tts}, time.Hour, func(string) bool { return false }, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err := e.Restore(ctx); err != nil {
			t.Fatal(err)
		}
		return e
	}
	engine := makeEngine(blobs)
	p := &provider.HTTP{Client: model.Client(), STT: map[string]provider.STT{"stt-1": {URL: model.URL + "/stt", Timeout: time.Second}}, TTS: map[string]provider.TTS{"tts-1": {URL: model.URL + "/tts", Timeout: time.Second, Profiles: map[string]provider.Options{"voice": {Seed: 1, CFGValue: 2, InferenceTimesteps: 20}}}}}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(26<<20), grpc.MaxSendMsgSize(65<<20))
	pb.RegisterVoiceGatewayServer(server, New(engine, p, map[string]string{"voice": "revision"}, 10*time.Second))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(26<<20), grpc.MaxCallRecvMsgSize(65<<20)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewVoiceGatewayClient(conn)
	sttCtx := metadata.AppendToOutgoingContext(ctx, "idempotency-key", "stt-once")
	transcript, err := client.Transcribe(sttCtx, &pb.TranscribeRequest{Audio: testutil.WAV(), MediaType: "audio/wav"})
	if err != nil || transcript.GetText() != "Распознанный ответ" {
		t.Fatalf("STT: %+v %v", transcript, err)
	}
	ttsCtx := metadata.AppendToOutgoingContext(ctx, "idempotency-key", "tts-once")
	req := &pb.SynthesizeRequest{Text: "Привет", VoiceProfile: "voice", VoiceRevision: "revision"}
	synthesized, err := client.Synthesize(ttsCtx, req)
	if err != nil || len(synthesized.GetAudio()) != 46 {
		t.Fatalf("TTS: %+v %v", synthesized, err)
	}
	if _, err = client.Synthesize(ttsCtx, req); err != nil || ttsCalls.Load() != 1 {
		t.Fatalf("duplicate model call: %v", err)
	}
	// A real protobuf request larger than gRPC's default 4 MiB must pass.
	large := make([]byte, 5<<20)
	copy(large, []byte("OggS"))
	if _, err = client.Transcribe(ctx, &pb.TranscribeRequest{Audio: large, MediaType: "audio/ogg"}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	engine.Metrics(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(recorder.Body.String(), "voice_route_selections_total{operation=\"stt\",model_instance=\"stt-1\"} 2") {
		t.Fatal(recorder.Body.String())
	}
	server.Stop()
	engine.Wait()
	blobs.Close()
	reopened, err := journal.OpenBlobs(dir, 128<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := makeEngine(reopened)
	direct := New(restored, p, map[string]string{"voice": "revision"}, time.Second)
	incoming := metadata.NewIncomingContext(ctx, metadata.Pairs("idempotency-key", "tts-once"))
	after, err := direct.Synthesize(incoming, req)
	if err != nil || after.RequestId != synthesized.RequestId || ttsCalls.Load() != 1 {
		t.Fatalf("restart lost durable result: %v", err)
	}
}
