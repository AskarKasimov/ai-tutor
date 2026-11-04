package translator

import (
	"context"
	"strings"
	"testing"
	"time"

	llmgatewayv1 "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc"
)

type finishGateway struct {
	llmgatewayv1.GatewayClient
	reason string
}

func (g finishGateway) Complete(context.Context, *llmgatewayv1.CompleteRequest, ...grpc.CallOption) (*llmgatewayv1.CompleteResponse, error) {
	return &llmgatewayv1.CompleteResponse{CompletionJson: []byte(`{"choices":[{"message":{"content":"x"},"finish_reason":"` + g.reason + `"}]}`)}, nil
}

func TestArbitraryFinishReasonStaysOutOfTheTranslationError(t *testing.T) {
	const invented = "PRIVATE_TEXT"
	_, err := NewGateway(finishGateway{reason: invented}, time.Second).Complete(context.Background(), "system", "prompt")
	if err == nil || strings.Contains(err.Error(), invented) {
		t.Fatalf("error = %v", err)
	}
}
