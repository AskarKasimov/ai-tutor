// Package llmgateway adapts the legacy OpenAI-shaped learning model clients to
// the gateway gRPC contract. It intentionally drops the clients' model field.
package llmgateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Transport struct {
	Gateway    pb.GatewayClient
	Profile    string
	Complexity pb.Complexity
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 1<<20 {
		return nil, http.ErrBodyNotAllowed
	}
	var wire struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Urgent              bool     `json:"urgent"`
		Temperature         *float64 `json:"temperature"`
		TopP                *float64 `json:"top_p"`
		PresencePenalty     *float64 `json:"presence_penalty"`
		FrequencyPenalty    *float64 `json:"frequency_penalty"`
		Seed                *int64   `json:"seed"`
		MaxTokens           *int64   `json:"max_tokens"`
		MaxCompletionTokens *int64   `json:"max_completion_tokens"`
		Stop                []string `json:"stop"`
		ResponseFormat      *struct {
			Type string `json:"type"`
		} `json:"response_format"`
		ReasoningEffort *string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	messages := make([]*pb.Message, 0, len(wire.Messages))
	for _, m := range wire.Messages {
		role := map[string]pb.Role{"system": pb.Role_ROLE_SYSTEM, "user": pb.Role_ROLE_USER, "assistant": pb.Role_ROLE_ASSISTANT}[m.Role]
		if role == pb.Role_ROLE_UNSPECIFIED {
			return nil, errors.New("invalid gateway request")
		}
		content := m.Content
		messages = append(messages, &pb.Message{Role: role, Content: &content})
	}
	parameters := &pb.GenerationParameters{Temperature: wire.Temperature, TopP: wire.TopP, PresencePenalty: wire.PresencePenalty, FrequencyPenalty: wire.FrequencyPenalty, Seed: wire.Seed, MaxTokens: wire.MaxTokens, MaxCompletionTokens: wire.MaxCompletionTokens}
	if len(wire.Stop) > 0 {
		parameters.Stop = &pb.StopSequences{Values: wire.Stop}
	}
	if wire.ResponseFormat != nil {
		f := map[string]pb.ResponseFormat{"text": pb.ResponseFormat_RESPONSE_FORMAT_TEXT, "json_object": pb.ResponseFormat_RESPONSE_FORMAT_JSON_OBJECT}[wire.ResponseFormat.Type]
		if f == pb.ResponseFormat_RESPONSE_FORMAT_UNSPECIFIED {
			return nil, errors.New("invalid gateway request")
		}
		parameters.ResponseFormat = &f
	}
	if wire.ReasoningEffort != nil {
		e := map[string]pb.ReasoningEffort{"low": pb.ReasoningEffort_REASONING_EFFORT_LOW, "medium": pb.ReasoningEffort_REASONING_EFFORT_MEDIUM, "high": pb.ReasoningEffort_REASONING_EFFORT_HIGH}[*wire.ReasoningEffort]
		if e == pb.ReasoningEffort_REASONING_EFFORT_UNSPECIFIED {
			return nil, errors.New("invalid gateway request")
		}
		parameters.ReasoningEffort = &e
	}
	result, err := t.Gateway.Complete(req.Context(), &pb.CompleteRequest{Messages: messages, Complexity: t.Complexity, Urgent: wire.Urgent, Parameters: parameters, RoutingProfile: t.Profile})
	if err != nil {
		code := http.StatusServiceUnavailable
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.InvalidArgument:
				code = http.StatusBadRequest
			case codes.ResourceExhausted:
				code = http.StatusTooManyRequests
			case codes.DeadlineExceeded:
				code = http.StatusGatewayTimeout
			case codes.Canceled:
				code = 499
			}
		}
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"message": status.Convert(err).Message()}})
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(b)), Request: req}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(result.CompletionJson)), Request: req}, nil
}

var _ http.RoundTripper = Transport{}
