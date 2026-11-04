package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const testTraceID = "0123456789abcdef0123456789abcdef"

func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(NewLogger(&output, level))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &output
}

func TestNewLoggerUsesTheSharedFieldNames(t *testing.T) {
	var output bytes.Buffer
	NewLogger(&output, slog.LevelInfo).Warn("event")
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["level"] != "warn" || entry["msg"] != "event" || entry["ts"] == nil {
		t.Fatalf("entry = %v", entry)
	}
}

func TestTraceIDFromParentAcceptsOnlyAValidTrace(t *testing.T) {
	for value, want := range map[string]string{
		"00-" + testTraceID + "-0123456789abcdef-01": testTraceID,
		"": "", "garbage": "",
		"00-00000000000000000000000000000000-0123456789abcdef-01": "",
		"00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-0123456789abcdef-01": "",
	} {
		if got := traceIDFromParent(value); got != want {
			t.Errorf("traceIDFromParent(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestLogGRPCCallLinksTheTraceToTheGatewayRequestWithoutContent(t *testing.T) {
	const text = "ответ студента"
	output := captureLogs(t, slog.LevelDebug)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("traceparent", "00-"+testTraceID+"-0123456789abcdef-01"))
	in := &pb.CompleteRequest{Complexity: pb.Complexity_COMPLEXITY_LOW, Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: proto.String(text)}}}
	logGRPCCall(ctx, in, "gateway-1", status.Error(codes.Unavailable, "down"), time.Now())
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["trace_id"] != testTraceID || entry["gateway_request_id"] != "gateway-1" || entry["level"] != "error" || entry["grpc_code"] != "Unavailable" {
		t.Fatalf("entry = %v", entry)
	}
	if bytes.Contains(output.Bytes(), []byte(text)) {
		t.Fatalf("the message text reached the log: %s", output)
	}
}

func TestLogHTTPKeepsProbesAtDebug(t *testing.T) {
	output := captureLogs(t, slog.LevelInfo)
	handler := logHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if output.Len() != 0 {
		t.Fatalf("a probe reached the info log: %s", output)
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if output.Len() == 0 {
		t.Fatal("a completion request was not logged")
	}
}

func TestConfigValidationRejectsAnUnknownLogLevel(t *testing.T) {
	config := Config{
		KafkaBrokers: []string{"localhost:9092"},
		Pools:        map[string][]string{"low": {"model-a"}, "medium": {"model-a"}, "high": {"model-a"}},
		Instances:    []InstanceConfig{{ID: "model-a", BaseURL: "http://localhost:8000/v1", Model: "served-model", MaxInFlight: 1, ContextWindow: 8192}},
	}
	config.defaults()
	if config.LogLevel != "info" {
		t.Fatalf("default log level = %q", config.LogLevel)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	config.LogLevel = "loud"
	if err := config.Validate(); err == nil {
		t.Fatal("an unknown log level was accepted")
	}
}
