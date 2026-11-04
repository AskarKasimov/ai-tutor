package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/logx"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const secretAnswer = "ответ студента"

func callThroughInterceptor(t *testing.T, level zapcore.Level, invokeErr error) (*observer.ObservedLogs, metadata.MD) {
	t.Helper()
	core, logs := observer.New(level)
	ctx := logx.WithTrace(logx.With(context.Background(), zap.New(core)), "0123456789abcdef0123456789abcdef")
	request := &pb.CompleteRequest{
		Complexity: pb.Complexity_COMPLEXITY_HIGH,
		Messages:   []*pb.Message{{Role: pb.Role_ROLE_USER, Content: proto.String(secretAnswer)}},
	}
	response := &pb.CompleteResponse{}
	var outgoing metadata.MD
	err := ClientInterceptor(ctx, "/Complete", request, response, nil, func(ctx context.Context, _ string, _, reply any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		outgoing, _ = metadata.FromOutgoingContext(ctx)
		if invokeErr != nil {
			return invokeErr
		}
		reply.(*pb.CompleteResponse).RequestId = "gateway-1"
		reply.(*pb.CompleteResponse).CompletionJson = []byte(secretAnswer)
		return nil
	})
	if !errors.Is(err, invokeErr) {
		t.Fatalf("error = %v", err)
	}
	return logs, outgoing
}

func TestClientInterceptorLogsTheCallWithoutItsContentAtInfo(t *testing.T) {
	logs, outgoing := callThroughInterceptor(t, zap.InfoLevel, nil)
	if !strings.HasPrefix(outgoing.Get("traceparent")[0], "00-0123456789abcdef0123456789abcdef-") {
		t.Fatalf("traceparent = %v", outgoing.Get("traceparent"))
	}
	if logs.Len() != 1 || logs.All()[0].Message != "llm call" {
		t.Fatalf("logs = %v", logs.All())
	}
	fields := logs.All()[0].ContextMap()
	if fields["gateway_request_id"] != "gateway-1" || fields["complexity"] != "COMPLEXITY_HIGH" {
		t.Fatalf("fields = %v", fields)
	}
	for key, value := range fields {
		if text, ok := value.(string); ok && strings.Contains(text, secretAnswer) {
			t.Fatalf("field %s leaked the content at info", key)
		}
	}
}

func TestClientInterceptorWritesContentOnlyAtDebug(t *testing.T) {
	logs, _ := callThroughInterceptor(t, zap.DebugLevel, nil)
	content := logs.FilterMessage("llm call content").All()
	if len(content) != 1 {
		t.Fatalf("logs = %v", logs.All())
	}
	fields := content[0].ContextMap()
	if !strings.Contains(fields["prompt"].(string), secretAnswer) || !strings.Contains(fields["response"].(string), secretAnswer) {
		t.Fatalf("fields = %v", fields)
	}
}

func TestSafeReasonDescribesTheShapeWithoutTheModelText(t *testing.T) {
	var strict struct {
		Feedback []string `json:"feedback"`
	}
	decoder := json.NewDecoder(strings.NewReader(`{"` + secretAnswer + `":1}`))
	decoder.DisallowUnknownFields()
	unknown := decoder.Decode(&strict)
	mismatch := json.Unmarshal([]byte(`{"feedback":"`+secretAnswer+`"}`), &strict)
	syntax := json.Unmarshal([]byte(secretAnswer), &strict)
	for _, tc := range []struct {
		err  error
		want string
	}{
		{unknown, "answer failed validation"},
		{mismatch, "field feedback has type string, want []string"},
		{syntax, "malformed JSON at offset 1"},
		{fmt.Errorf("unknown verdict %q", secretAnswer), "answer failed validation"},
	} {
		if got := SafeReason(tc.err); got != tc.want {
			t.Errorf("SafeReason(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestClientInterceptorWarnsAboutAFailedCall(t *testing.T) {
	logs, _ := callThroughInterceptor(t, zap.InfoLevel, errors.New("gateway down"))
	if logs.Len() != 1 || logs.All()[0].Level != zap.WarnLevel || logs.All()[0].ContextMap()["error"] != "gateway down" {
		t.Fatalf("logs = %v", logs.All())
	}
}
