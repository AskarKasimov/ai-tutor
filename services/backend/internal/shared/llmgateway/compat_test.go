package llmgateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc"
)

type gatewayFake struct{ got *pb.CompleteRequest }

func (g *gatewayFake) Complete(_ context.Context, req *pb.CompleteRequest, _ ...grpc.CallOption) (*pb.CompleteResponse, error) {
	g.got = req
	return &pb.CompleteResponse{CompletionJson: []byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`)}, nil
}

func TestTransportRoutesLegacyPromptThroughConfiguredGatewayProfile(t *testing.T) {
	gateway := &gatewayFake{}
	transport := Transport{Gateway: gateway, Profile: "learning-assessment", Complexity: pb.Complexity_COMPLEXITY_HIGH}
	request, _ := http.NewRequest(http.MethodPost, "http://gateway.invalid/v1/chat/completions", strings.NewReader(`{"model":"must-not-route","messages":[{"role":"system","content":"prompt"},{"role":"user","content":"data"}],"response_format":{"type":"json_object"},"temperature":0,"max_tokens":2048,"reasoning_effort":"medium"}`))
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"finish_reason":"stop"`) {
		t.Fatalf("response=%d %s", response.StatusCode, body)
	}
	if gateway.got == nil || gateway.got.RoutingProfile != "learning-assessment" || gateway.got.Complexity != pb.Complexity_COMPLEXITY_HIGH || len(gateway.got.Messages) != 2 || gateway.got.Parameters == nil || gateway.got.Parameters.GetMaxTokens() != 2048 {
		t.Fatalf("unexpected gateway request: %+v", gateway.got)
	}
}
