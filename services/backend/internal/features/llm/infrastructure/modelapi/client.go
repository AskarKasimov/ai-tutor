package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client bounds in-flight requests to one per endpoint; waiting respects context.
// Consumers see a single model even when up to four servers are configured.
type Client struct {
	slots        chan string
	client       *http.Client
	model, token string
	timeout      time.Duration
}

func New(client *http.Client, endpoints []string, model, token string, timeout time.Duration) *Client {
	c := &Client{slots: make(chan string, len(endpoints)), client: client, model: model, token: token, timeout: timeout}
	for _, endpoint := range endpoints {
		c.slots <- strings.TrimRight(endpoint, "/")
	}
	return c
}

func (c *Client) Complete(ctx context.Context, system, input string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var endpoint string
	select {
	case endpoint = <-c.slots:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { c.slots <- endpoint }()
	payload, err := json.Marshal(map[string]any{
		"model": c.model, "temperature": 0, "max_tokens": 4096, "reasoning_effort": "low",
		"messages":        []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": input}},
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("model request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&result); err != nil {
		return "", fmt.Errorf("model response: %w", err)
	}
	if len(result.Choices) != 1 || result.Choices[0].FinishReason != "stop" || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("incomplete model response")
	}
	return result.Choices[0].Message.Content, nil
}
