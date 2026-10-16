package gateway

import (
	"bytes"
	"context"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func grpcRequest() *pb.CompleteRequest {
	text := ""
	return &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &text}}, Complexity: pb.Complexity_COMPLEXITY_LOW}
}
func TestGRPCContract(t *testing.T) {
	tests := []struct {
		name   string
		change func(*pb.CompleteRequest)
		code   string
	}{
		{"missing complexity", func(r *pb.CompleteRequest) { r.Complexity = 0 }, "invalid_complexity"},
		{"unknown role", func(r *pb.CompleteRequest) { r.Messages[0].Role = pb.Role(99) }, "invalid_messages"},
		{"missing content", func(r *pb.CompleteRequest) { r.Messages[0].Content = nil }, "invalid_messages"},
		{"NaN", func(r *pb.CompleteRequest) { v := math.NaN(); r.Parameters = &pb.GenerationParameters{Temperature: &v} }, "invalid_parameter"},
		{"empty stop", func(r *pb.CompleteRequest) { r.Parameters = &pb.GenerationParameters{Stop: &pb.StopSequences{}} }, "invalid_parameter"},
		{"zero budget", func(r *pb.CompleteRequest) { v := int64(0); r.Parameters = &pb.GenerationParameters{MaxTokens: &v} }, "invalid_parameter"},
		{"conflicting budgets", func(r *pb.CompleteRequest) {
			v := int64(1)
			r.Parameters = &pb.GenerationParameters{MaxTokens: &v, MaxCompletionTokens: &v}
		}, "unsupported_parameter"},
		{"unknown format", func(r *pb.CompleteRequest) {
			v := pb.ResponseFormat(99)
			r.Parameters = &pb.GenerationParameters{ResponseFormat: &v}
		}, "unsupported_parameter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := grpcRequest()
			tt.change(r)
			_, err := requestFromProto(r)
			if err == nil {
				t.Fatal("expected validation error")
			}
			got := err.(statusError).body.Error.Code
			if got != tt.code {
				t.Fatalf("code=%s, want %s", got, tt.code)
			}
		})
	}
}
func TestGRPCOptionalPresenceAndHash(t *testing.T) {
	base := grpcRequest()
	a, err := requestFromProto(base)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	base.Parameters = &pb.GenerationParameters{Temperature: &zero}
	b, err := requestFromProto(base)
	if err != nil {
		t.Fatal(err)
	}
	if a.CanonicalHash() == b.CanonicalHash() {
		t.Fatal("explicit zero temperature was lost")
	}
	base.Parameters = &pb.GenerationParameters{Stop: &pb.StopSequences{Values: []string{"END"}}}
	c, err := requestFromProto(base)
	if err != nil {
		t.Fatal(err)
	}
	http, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":""}],"complexity":"low","stop":"END"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.CanonicalHash() != http.CanonicalHash() {
		t.Fatal("HTTP stop string and protobuf stop differ")
	}
}
func TestGRPCStatusDetails(t *testing.T) {
	err := grpcError(apiErr(409, "idempotency_error", "idempotency_conflict", "conflict"), "request-1")
	st, _ := status.FromError(err)
	if st.Code() != codes.AlreadyExists || len(st.Details()) != 1 {
		t.Fatalf("status=%v", st)
	}
}

func TestGRPCLargeSavedResponseAndSendLimit(t *testing.T) {
	for _, tt := range []struct {
		name      string
		sendLimit int
		wantCode  codes.Code
	}{{"large completion", 6 << 20, codes.OK}, {"send limit", 4 << 20, codes.ResourceExhausted}} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{KafkaBrokers: []string{"localhost:9092"}, MaxConnections: 2, MaxBodyBytes: 1 << 20, GRPCMaxSendBytes: tt.sendLimit, Pools: map[string][]string{"low": {"a"}}, Instances: []InstanceConfig{{ID: "a", Model: "a", MaxInFlight: 1, ContextWindow: 8192, SupportedResponseFormats: []string{"text"}}}}
			s, err := NewService(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			req, err := requestFromProto(grpcRequest())
			if err != nil {
				t.Fatal(err)
			}
			key := "large-result"
			id := stableJobID("internal", key)
			body := []byte(`{"choices":"` + strings.Repeat("x", 5<<20) + `"}`)
			s.mu.Lock()
			s.ready = true
			s.jobs[id] = &durableJob{ID: id, ClientID: "internal", IdempotencyKey: key, Hash: req.CanonicalHash(), Request: req, Status: "succeeded", Response: body, SubmittedAt: time.Now()}
			s.keys[identity("internal", key)] = id
			s.mu.Unlock()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer(grpc.MaxSendMsgSize(tt.sendLimit))
			pb.RegisterGatewayServer(server, NewGRPCServer(s))
			go func() { _ = server.Serve(listener) }()
			defer server.Stop()
			conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(6<<20)))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", key), 5*time.Second)
			defer cancel()
			got, err := pb.NewGatewayClient(conn).Complete(ctx, grpcRequest())
			if status.Code(err) != tt.wantCode {
				t.Fatalf("status=%v, want %v", status.Code(err), tt.wantCode)
			}
			if tt.wantCode == codes.OK && !bytes.Equal(got.CompletionJson, body) {
				t.Fatal("saved JSON bytes changed")
			}
			if tt.wantCode != codes.OK {
				st, _ := status.FromError(err)
				detail := st.Details()[0].(*errdetails.ErrorInfo)
				if detail.Reason != "response_too_large" {
					t.Fatalf("reason=%s", detail.Reason)
				}
				s.mu.Lock()
				stillSaved := s.jobs[id].Status == "succeeded"
				s.mu.Unlock()
				if !stillSaved {
					t.Fatal("send limit changed saved job")
				}
			}
		})
	}
}

func TestSharedConnectionLimitAcrossTransports(t *testing.T) {
	cfg := Config{KafkaBrokers: []string{"localhost:9092"}, MaxConnections: 1, MaxBodyBytes: 1 << 20, Pools: map[string][]string{"low": {"a"}}, Instances: []InstanceConfig{{ID: "a", Model: "a", MaxInFlight: 1, ContextWindow: 8192, SupportedResponseFormats: []string{"text"}}}}
	s, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	s.connections <- struct{}{}
	defer func() { <-s.connections }()
	_, _, err = s.Complete(context.Background(), mustRequest(t), "", "grpc", false, nil)
	if status.Code(grpcError(err, "id")) != codes.ResourceExhausted {
		t.Fatalf("grpc limit error=%v", err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":""}],"complexity":"low"}`))
	s.Handler().ServeHTTP(response, request)
	if response.Code != 429 || !strings.Contains(response.Body.String(), "connection_limit") {
		t.Fatalf("http limit=%d %s", response.Code, response.Body.String())
	}
}
func mustRequest(t *testing.T) Request {
	t.Helper()
	r, e := requestFromProto(grpcRequest())
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestGRPCRetryInfo(t *testing.T) {
	st, _ := status.FromError(grpcErrorWithRetry(apiErr(429, "rate_limit_error", "upstream_rate_limit", "retry"), "id", 3))
	if st.Code() != codes.ResourceExhausted || len(st.Details()) != 2 {
		t.Fatalf("details=%v", st)
	}
	retry := st.Details()[1].(*errdetails.RetryInfo)
	if retry.RetryDelay.AsDuration() != 3*time.Second {
		t.Fatalf("retry=%v", retry.RetryDelay)
	}
}
