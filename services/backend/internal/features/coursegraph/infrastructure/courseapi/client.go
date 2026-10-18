package courseapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
)

type Client struct {
	base   string
	client *http.Client
	// poll is how often a running build's progress is read.
	poll time.Duration
}

func New(base string) *Client {
	return &Client{strings.TrimRight(base, "/"), &http.Client{}, 2 * time.Second}
}

func (c *Client) Build(ctx context.Context, buildID string, materials []coursegraph.Material, report func(coursegraph.BuildProgress)) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"materials": materials})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/graph/build", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Build-ID", buildID)
	// The build call is synchronous: progress is read beside it and stops
	// before Build returns, so report is never called after that.
	pollCtx, stop := context.WithCancel(ctx)
	var polling sync.WaitGroup
	polling.Go(func() { c.pollProgress(pollCtx, buildID, report) })
	defer polling.Wait()
	defer stop()
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

// pollProgress reports the progress course-graph shows for this build. A
// missing, failed or foreign answer is skipped: progress is only a hint.
func (c *Client) pollProgress(ctx context.Context, buildID string, report func(coursegraph.BuildProgress)) {
	ticker := time.NewTicker(c.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if p, ok := c.progress(ctx, buildID); ok && ctx.Err() == nil {
			report(p)
		}
	}
}

func (c *Client) progress(ctx context.Context, buildID string) (coursegraph.BuildProgress, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/graph/build/progress", nil)
	if err != nil {
		return coursegraph.BuildProgress{}, false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return coursegraph.BuildProgress{}, false
	}
	defer resp.Body.Close()
	var body struct {
		BuildID string `json:"build_id"`
		coursegraph.BuildProgress
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body) != nil || body.BuildID != buildID {
		return coursegraph.BuildProgress{}, false
	}
	return body.BuildProgress, true
}
