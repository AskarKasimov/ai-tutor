package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Circuit struct {
	mu        sync.Mutex
	failures  int
	openUntil time.Time
	probing   bool
}

func (c *Circuit) acquire(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures < 3 {
		return true
	}
	if now.Before(c.openUntil) || c.probing {
		return false
	}
	c.probing = true
	return true
}
func (c *Circuit) release(ok bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		c.failures = 0
		c.probing = false
		c.openUntil = time.Time{}
		return
	}
	c.failures++
	c.probing = false
	if c.failures >= 3 {
		c.openUntil = now.Add(15 * time.Second)
	}
}

type UpstreamResult struct {
	Body       []byte
	Status     int
	RetryAfter time.Duration
}
type UpstreamFailure struct {
	Status     int
	Code       string
	Err        error
	BeforeSend bool
	RetryAfter time.Duration
}

func (e *UpstreamFailure) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

type Caller struct {
	cfg      Config
	client   *http.Client
	circuits map[string]*Circuit
	mu       sync.Mutex
}

func NewCaller(cfg Config) *Caller {
	return &Caller{cfg: cfg, client: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, MaxIdleConns: 128, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: cfg.UpstreamTimeout}, Timeout: cfg.UpstreamTimeout}, circuits: make(map[string]*Circuit)}
}
func (c *Caller) circuit(id string) *Circuit {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.circuits[id] == nil {
		c.circuits[id] = &Circuit{}
	}
	return c.circuits[id]
}

func (c *Caller) call(ctx context.Context, in InstanceConfig, req Request, requestID string, probeReserved bool) (UpstreamResult, *UpstreamFailure) {
	cb := c.circuit(in.ID)
	if !probeReserved && !cb.acquire(time.Now()) {
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_circuit_open", Err: fmt.Errorf("instance temporarily unavailable"), BeforeSend: true}
	}
	params := make(map[string]json.RawMessage, len(req.Parameters))
	for k, v := range req.Parameters {
		params[k] = v
	}
	var fields map[string]any
	_ = json.Unmarshal(mustJSON(params), &fields)
	fields["model"] = in.Model
	fields["messages"] = req.Messages
	body, err := json.Marshal(fields)
	if err != nil {
		cb.release(false, time.Now())
		return UpstreamResult{}, &UpstreamFailure{Code: "invalid_upstream_request", Err: err, BeforeSend: true}
	}
	endpoint := strings.TrimRight(in.BaseURL, "/") + "/v1/chat/completions"
	if strings.HasSuffix(strings.TrimRight(in.BaseURL, "/"), "/v1") {
		endpoint = strings.TrimRight(in.BaseURL, "/") + "/chat/completions"
	}
	callCtx, cancel := context.WithTimeout(ctx, minDuration(c.cfg.UpstreamTimeout, in.Timeout))
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		cb.release(false, time.Now())
		return UpstreamResult{}, &UpstreamFailure{Code: "invalid_upstream_url", Err: err, BeforeSend: true}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if in.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+in.Token)
	}
	httpReq.Header.Set("X-Request-ID", requestID)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			cb.releaseReservation()
			return UpstreamResult{}, &UpstreamFailure{Code: "request_canceled", Err: err}
		}
		var ne net.Error
		before := strings.Contains(err.Error(), "connect: ") || strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "network is unreachable")
		if errorsAs(err, &ne) && !ne.Timeout() {
			before = before || strings.Contains(err.Error(), "dial")
		}
		cb.release(false, time.Now())
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_transport_error", Err: err, BeforeSend: before}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			cb.releaseReservation()
			return UpstreamResult{}, &UpstreamFailure{Code: "request_canceled", Err: err}
		}
		cb.release(false, time.Now())
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_response_error", Err: err}
	}
	if resp.StatusCode >= 500 {
		cb.release(false, time.Now())
	} else if resp.StatusCode < 500 {
		cb.release(true, time.Now())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d := parseRetryAfter(resp.Header.Get("Retry-After"))
		return UpstreamResult{}, &UpstreamFailure{Status: resp.StatusCode, Code: "upstream_error", Err: fmt.Errorf("upstream returned HTTP %d", resp.StatusCode), RetryAfter: d}
	}
	if len(raw) > 32<<20 {
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_response_too_large", Err: fmt.Errorf("upstream response exceeded 32 MiB")}
	}
	var completion map[string]json.RawMessage
	if json.Unmarshal(raw, &completion) != nil {
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_invalid_response", Err: fmt.Errorf("upstream response is not valid JSON")}
	}
	var shape struct {
		ID      string            `json:"id"`
		Object  string            `json:"object"`
		Created int64             `json:"created"`
		Choices []json.RawMessage `json:"choices"`
	}
	if json.Unmarshal(raw, &shape) != nil || shape.ID == "" || shape.Object != "chat.completion" || shape.Created <= 0 || len(shape.Choices) == 0 {
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_invalid_response", Err: fmt.Errorf("upstream returned an incomplete chat completion")}
	}
	completion["model"], _ = json.Marshal(in.Model)
	out, err := json.Marshal(completion)
	if err != nil {
		return UpstreamResult{}, &UpstreamFailure{Code: "upstream_invalid_response", Err: err}
	}
	return UpstreamResult{Body: out, Status: resp.StatusCode}, nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func minDuration(a, b time.Duration) time.Duration {
	if b <= 0 || b > a {
		return a
	}
	return b
}
func parseRetryAfter(v string) time.Duration {
	if n, e := strconv.Atoi(v); e == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, e := http.ParseTime(v); e == nil && t.After(time.Now()) {
		return time.Until(t)
	}
	return 0
}

var errorsAs = errors.As
