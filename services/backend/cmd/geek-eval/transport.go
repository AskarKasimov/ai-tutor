package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	llm "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

var errPreview = errors.New("preview: no model called")

type call struct {
	Label    string          `json:"label"`
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response,omitempty"`
	Seconds  float64         `json:"seconds"`
	Error    string          `json:"error,omitempty"`
}

// This explicit evaluation-only transport uses the endpoint supplied by the
// operator. Application traffic still goes through the production LLM gateway.
// Gateway's real prompt builder, parser and retry logic are reused unchanged.
type transport struct {
	endpoint, model, reasoning, label string
	preview                           bool
	http                              *http.Client
	calls                             []call
}

func (t *transport) Complete(ctx context.Context, r *llm.CompleteRequest, _ ...grpc.CallOption) (*llm.CompleteResponse, error) {
	request, _ := protojson.Marshal(r)
	c := call{Label: t.label, Request: request}
	if t.preview {
		t.calls = append(t.calls, c)
		return nil, errPreview
	}
	messages := []map[string]string{}
	for _, m := range r.Messages {
		role := strings.ToLower(strings.TrimPrefix(m.Role.String(), "ROLE_"))
		messages = append(messages, map[string]string{"role": role, "content": m.GetContent()})
	}
	effort := t.reasoning
	if effort == "production" {
		effort = strings.ToLower(strings.TrimPrefix(r.GetParameters().GetReasoningEffort().String(), "REASONING_EFFORT_"))
	}
	body := map[string]any{
		"model": t.model, "messages": messages,
		"temperature":      r.GetParameters().GetTemperature(),
		"response_format":  map[string]string{"type": "json_object"},
		"reasoning_effort": effort,
		"max_tokens":       4096,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	// Store the actual experimental request, including the chosen reasoning
	// setting; a preview stores the unmodified production gateway request.
	c.Request = encoded
	started := time.Now()
	response, err := t.post(ctx, encoded)
	c.Seconds = time.Since(started).Seconds()
	c.Response = response
	if err != nil {
		c.Error = err.Error()
	}
	t.calls = append(t.calls, c)
	if err != nil {
		return nil, err
	}
	return &llm.CompleteResponse{RequestId: fmt.Sprintf("eval-%d", len(t.calls)), CompletionJson: response}, nil
}

func (t *transport) post(ctx context.Context, body []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model returned HTTP %d: %.500s", resp.StatusCode, data)
	}
	if !json.Valid(data) {
		return nil, errors.New("model endpoint returned invalid JSON")
	}
	return data, nil
}
