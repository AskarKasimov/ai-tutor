package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	llmgatewayv1 "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
)

// Gateway is the Completer over the LLM gateway (llmgateway.v1.Gateway/Complete):
// the client names only the complexity, never the model.
type Gateway struct {
	client  llmgatewayv1.GatewayClient
	timeout time.Duration
}

func NewGateway(client llmgatewayv1.GatewayClient, timeout time.Duration) *Gateway {
	return &Gateway{client: client, timeout: timeout}
}

func (g *Gateway) Complete(ctx context.Context, system, prompt string) (string, error) {
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}
	response, err := g.client.Complete(ctx, &llmgatewayv1.CompleteRequest{
		Messages: []*llmgatewayv1.Message{
			{Role: llmgatewayv1.Role_ROLE_SYSTEM, Content: proto.String(system)},
			{Role: llmgatewayv1.Role_ROLE_USER, Content: proto.String(prompt)},
		},
		// Every instance serves low, and the gateway gives it to the free one
		// expected to answer soonest (decision of 18.10.2026).
		Complexity: llmgatewayv1.Complexity_COMPLEXITY_LOW,
		Parameters: &llmgatewayv1.GenerationParameters{
			Temperature:    proto.Float64(0.2),
			ResponseFormat: llmgatewayv1.ResponseFormat_RESPONSE_FORMAT_JSON_OBJECT.Enum(),
			// A translation needs no reasoning; it only slows thinking models.
			ReasoningEffort: llmgatewayv1.ReasoningEffort_REASONING_EFFORT_LOW.Enum(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("call LLM gateway: %w", err)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(response.GetCompletionJson(), &completion); err != nil || len(completion.Choices) == 0 {
		return "", errors.New("LLM gateway returned no choices")
	}
	if reason := completion.Choices[0].FinishReason; reason != "" && reason != "stop" {
		return "", fmt.Errorf("incomplete translation response: %s", reason)
	}
	return completion.Choices[0].Message.Content, nil
}
