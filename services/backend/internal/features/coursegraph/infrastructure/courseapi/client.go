package courseapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
)

type Client struct {
	base   string
	client *http.Client
}

func New(base string) *Client {
	return &Client{strings.TrimRight(base, "/"), &http.Client{}}
}
func (c *Client) Build(ctx context.Context, materials []coursegraph.Material) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"materials": materials})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/graph/build", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graph HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024*1024 || !json.Valid(data) {
		return nil, fmt.Errorf("invalid graph payload")
	}
	return data, nil
}
