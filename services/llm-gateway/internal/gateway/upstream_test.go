package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCallerSendsConfiguredRequestAndNormalizesModel(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "without auth token"},
		{name: "with auth token", token: "upstream-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
					t.Errorf("request = %s %s, want POST /v1/chat/completions", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); tt.token == "" && got != "" {
					t.Errorf("Authorization = %q, want empty", got)
				} else if tt.token != "" && got != "Bearer "+tt.token {
					t.Errorf("Authorization = %q, want bearer token", got)
				}
				if got := r.Header.Get("X-Request-ID"); got != "request-123" {
					t.Errorf("X-Request-ID = %q, want request-123", got)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode upstream body: %v", err)
				}
				var model string
				if err := json.Unmarshal(body["model"], &model); err != nil || model != "served-model" {
					t.Errorf("upstream model = %q, want served-model (err=%v)", model, err)
				}
				if _, ok := body["messages"]; !ok {
					t.Error("upstream body has no messages")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"completion-1","object":"chat.completion","created":1,"model":"server-alias","choices":[{"index":0,"message":{"role":"assistant","content":"hello"}}]}`))
			}))
			defer server.Close()

			cfg := Config{UpstreamTimeout: 2 * time.Second}
			caller := NewCaller(cfg)
			defer caller.client.CloseIdleConnections()
			request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low","temperature":0.2}`))
			if err != nil {
				t.Fatal(err)
			}
			got, failure := caller.call(context.Background(), InstanceConfig{ID: "model-a", BaseURL: server.URL + "/v1", Model: "served-model", Token: tt.token, Timeout: 2 * time.Second}, request, "request-123", false)
			if failure != nil {
				t.Fatalf("call() failure = %v", failure)
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal(got.Body, &response); err != nil {
				t.Fatalf("decode completion: %v", err)
			}
			var model string
			if err := json.Unmarshal(response["model"], &model); err != nil || model != "served-model" {
				t.Fatalf("response model = %q, want served-model (err=%v)", model, err)
			}
		})
	}
}

func TestCallerPreservesLargeIntegerSeed(t *testing.T) {
	var gotSeed string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotSeed = string(body["seed"])
		_, _ = w.Write([]byte(`{"id":"completion-1","object":"chat.completion","created":1,"choices":[{}]}`))
	}))
	defer server.Close()
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low","seed":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	caller := NewCaller(Config{UpstreamTimeout: time.Second})
	defer caller.client.CloseIdleConnections()
	_, failure := caller.call(context.Background(), InstanceConfig{ID: "model-a", BaseURL: server.URL, Model: "served-model", Timeout: time.Second}, request, "request-123", false)
	if failure != nil {
		t.Fatalf("call() failure = %v", failure)
	}
	if gotSeed != "9007199254740993" {
		t.Fatalf("upstream seed = %s, want 9007199254740993", gotSeed)
	}
}

func TestCallerReturnsRetryAfterForUpstreamRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		http.Error(w, `{"error":"busy"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	caller := NewCaller(Config{UpstreamTimeout: time.Second})
	defer caller.client.CloseIdleConnections()
	_, failure := caller.call(context.Background(), InstanceConfig{ID: "model-a", BaseURL: server.URL, Model: "served-model", Timeout: time.Second}, request, "request-123", false)
	if failure == nil {
		t.Fatal("call() failure = nil, want upstream 429")
	}
	if failure.Status != http.StatusTooManyRequests || failure.RetryAfter != 7*time.Second {
		t.Fatalf("failure = (%d, %s), want (429, 7s)", failure.Status, failure.RetryAfter)
	}
}

func reserved(c *Circuit, now time.Time) bool {
	_, ok := c.reserve(now)
	return ok
}

func TestCircuitBreakerOpensAndAllowsOneHalfOpenProbe(t *testing.T) {
	var circuit Circuit
	now := time.Now()
	for range 3 {
		ticket, ok := circuit.reserve(now)
		if !ok || ticket.probe {
			t.Fatal("closed circuit rejected request or handed out a probe")
		}
		circuit.release(false, ticket, now)
	}
	if reserved(&circuit, now.Add(14*time.Second)) {
		t.Fatal("open circuit allowed request before reset timeout")
	}
	probeTime := now.Add(15 * time.Second)
	probeTicket, ok := circuit.reserve(probeTime)
	if !ok || !probeTicket.probe {
		t.Fatal("circuit did not allow half-open probe after reset timeout")
	}
	if reserved(&circuit, probeTime) {
		t.Fatal("circuit allowed more than one half-open probe")
	}
	circuit.release(true, probeTicket, probeTime)
	if !reserved(&circuit, probeTime) {
		t.Fatal("successful probe did not close circuit")
	}
}

func TestCallerAddsTheInstanceBodyForTheRequestedReasoningEffort(t *testing.T) {
	var bodies []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	caller := NewCaller(Config{UpstreamTimeout: 2 * time.Second})
	defer caller.client.CloseIdleConnections()
	in := InstanceConfig{ID: "qwen", BaseURL: server.URL + "/v1", Model: "qwen", Timeout: 2 * time.Second,
		ReasoningEffortBody: map[string]map[string]json.RawMessage{"low": {"chat_template_kwargs": json.RawMessage(`{"enable_thinking":false}`)}}}
	for _, raw := range []string{
		`{"messages":[{"role":"user","content":"hi"}],"complexity":"low","reasoning_effort":"low"}`,
		`{"messages":[{"role":"user","content":"hi"}],"complexity":"low","reasoning_effort":"high"}`,
		`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`,
	} {
		request, err := ParseRequest([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, failure := caller.call(context.Background(), in, request, "r", false); failure != nil {
			t.Fatal(failure)
		}
	}
	if got := string(bodies[0]["chat_template_kwargs"]); got != `{"enable_thinking":false}` {
		t.Fatalf("low effort body = %q", got)
	}
	if _, ok := bodies[0]["reasoning_effort"]; !ok {
		t.Fatal("the requested effort itself is still sent")
	}
	for i, body := range bodies[1:] {
		if _, ok := body["chat_template_kwargs"]; ok {
			t.Fatalf("request %d without low effort got the low body", i+1)
		}
	}
}

func slowUpstream(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"x"}}]}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestCallerDoesNotCountTheJobDeadlineAsAnUpstreamFailure(t *testing.T) {
	server := slowUpstream(t, 300*time.Millisecond)
	caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
	defer caller.client.CloseIdleConnections()
	in := InstanceConfig{ID: "model-a", BaseURL: server.URL + "/v1", Model: "m", Timeout: 5 * time.Second}
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		_, failure := caller.call(ctx, in, request, "r", false)
		cancel()
		if failure == nil || !failure.JobDeadline {
			t.Fatalf("call %d failure = %+v, want JobDeadline", i, failure)
		}
		if !errors.Is(failure.Err, context.DeadlineExceeded) {
			t.Fatalf("call %d: the error lost context.DeadlineExceeded: %v", i, failure.Err)
		}
	}
	if !caller.circuit("model-a").available(time.Now()) {
		t.Fatal("the job deadline opened the breaker of a healthy instance")
	}
}

func TestCallerStillCountsTheInstanceTimeoutAsAFailure(t *testing.T) {
	server := slowUpstream(t, 300*time.Millisecond)
	caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
	defer caller.client.CloseIdleConnections()
	in := InstanceConfig{ID: "model-a", BaseURL: server.URL + "/v1", Model: "m", Timeout: 30 * time.Millisecond}
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		_, failure := caller.call(context.Background(), in, request, "r", false)
		if failure == nil || failure.Code != "upstream_transport_error" || !errors.Is(failure.Err, context.DeadlineExceeded) {
			t.Fatalf("call %d failure = %+v, want upstream_transport_error with DeadlineExceeded", i, failure)
		}
	}
	if caller.circuit("model-a").available(time.Now()) {
		t.Fatal("three instance timeouts must open the breaker")
	}
}

func TestJobDeadlineReleasesTheHalfOpenProbe(t *testing.T) {
	server := slowUpstream(t, 300*time.Millisecond)
	caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
	defer caller.client.CloseIdleConnections()
	in := InstanceConfig{ID: "model-a", BaseURL: server.URL + "/v1", Model: "m", Timeout: 5 * time.Second}
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	cb := caller.circuit("model-a")
	past := time.Now().Add(-time.Minute)
	for range 3 {
		cb.release(false, circuitTicket{}, past)
	}
	if !reserved(cb, time.Now()) {
		t.Fatal("half-open probe was not granted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, failure := caller.call(ctx, in, request, "r", true)
	if failure == nil || !failure.JobDeadline {
		t.Fatalf("failure = %+v, want JobDeadline", failure)
	}
	if !reserved(cb, time.Now()) {
		t.Fatal("the probe stayed reserved after the job deadline; next probe is blocked")
	}
}

// lateFailTransport fails a call only some time after its context is done, the
// way a transport may report a timeout late. With body it fails the read of the
// response body instead of the request.
type lateFailTransport struct {
	after time.Duration
	body  bool
}

type lateFailBody struct {
	ctx   context.Context
	after time.Duration
}

func (b lateFailBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	time.Sleep(b.after)
	return 0, b.ctx.Err()
}
func (lateFailBody) Close() error { return nil }

func (lt lateFailTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if lt.body {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: lateFailBody{ctx: r.Context(), after: lt.after}, Request: r}, nil
	}
	<-r.Context().Done()
	time.Sleep(lt.after)
	return nil, r.Context().Err()
}

func TestInstanceTimeoutReportedAfterTheJobDeadlineStillCountsAsAFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		body bool
		code string
	}{{"request", false, "upstream_transport_error"}, {"response body", true, "upstream_response_error"}} {
		t.Run(tc.name, func(t *testing.T) {
			caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
			caller.client.Transport = lateFailTransport{after: 60 * time.Millisecond, body: tc.body}
			in := InstanceConfig{ID: "model-a", BaseURL: "http://upstream.invalid/v1", Model: "m", Timeout: 20 * time.Millisecond}
			request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
			if err != nil {
				t.Fatal(err)
			}
			for i := range 3 {
				// The instance timeout (20 ms) fires first; the job deadline (40 ms)
				// has passed too by the time the transport reports the failure.
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
				_, failure := caller.call(ctx, in, request, "r", false)
				cancel()
				if failure == nil || failure.Code != tc.code || !errors.Is(failure.Err, context.DeadlineExceeded) {
					t.Fatalf("call %d failure = %+v, want %s with DeadlineExceeded", i, failure, tc.code)
				}
			}
			if caller.circuit("model-a").available(time.Now()) {
				t.Fatal("instance timeouts reported after the job deadline did not open the breaker")
			}
		})
	}
}

// takeOverBreaker simulates three other failures that open the breaker and a
// later call that takes the half-open probe while a non-probe call is in flight.
func takeOverBreaker(t *testing.T, cb *Circuit) {
	t.Helper()
	past := time.Now().Add(-time.Minute)
	cb.mu.Lock()
	cb.failures = 3
	cb.openUntil = past
	cb.mu.Unlock()
	// The breaker opened and moved to a new generation.
	cb.generation++
	if ticket, ok := cb.reserve(time.Now()); !ok || !ticket.probe {
		t.Fatalf("reserve = (%v, %v), want the half-open probe", ok, ticket.probe)
	}
}

func TestNonProbeCallEndedByTheJobDeadlineKeepsTheProbeOfAnotherCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		body bool
	}{{"request", false}, {"response body", true}} {
		t.Run(tc.name, func(t *testing.T) {
			caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
			caller.client.Transport = lateFailTransport{body: tc.body}
			in := InstanceConfig{ID: "model-a", BaseURL: "http://upstream.invalid/v1", Model: "m", Timeout: 5 * time.Second}
			request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
			if err != nil {
				t.Fatal(err)
			}
			cb := caller.circuit("model-a")
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			done := make(chan *UpstreamFailure, 1)
			go func() { _, f := caller.call(ctx, in, request, "r", false); done <- f }()
			time.Sleep(30 * time.Millisecond)
			takeOverBreaker(t, cb)
			if failure := <-done; failure == nil {
				t.Fatal("call succeeded, want the job deadline")
			}
			if _, ok := cb.reserve(time.Now()); ok {
				t.Fatal("a non-probe call cut by the job deadline released the probe of another call: second probe granted")
			}
		})
	}
}

func TestNonProbeCallCanceledKeepsTheProbeOfAnotherCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		body bool
	}{{"request", false}, {"response body", true}} {
		t.Run(tc.name, func(t *testing.T) {
			caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
			caller.client.Transport = lateFailTransport{body: tc.body}
			in := InstanceConfig{ID: "model-a", BaseURL: "http://upstream.invalid/v1", Model: "m", Timeout: 5 * time.Second}
			request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
			if err != nil {
				t.Fatal(err)
			}
			cb := caller.circuit("model-a")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan *UpstreamFailure, 1)
			go func() { _, f := caller.call(ctx, in, request, "r", false); done <- f }()
			time.Sleep(30 * time.Millisecond)
			takeOverBreaker(t, cb)
			cancel()
			if failure := <-done; failure == nil || failure.Code != "request_canceled" {
				t.Fatalf("failure = %+v, want request_canceled", failure)
			}
			if _, ok := cb.reserve(time.Now()); ok {
				t.Fatal("a canceled non-probe call released the probe of another call: second probe granted")
			}
		})
	}
}

func TestNonProbeFailureKeepsTheProbeOfAnotherCall(t *testing.T) {
	proceed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-proceed
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
	defer caller.client.CloseIdleConnections()
	in := InstanceConfig{ID: "model-a", BaseURL: server.URL + "/v1", Model: "m", Timeout: 5 * time.Second}
	request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	cb := caller.circuit("model-a")
	done := make(chan *UpstreamFailure, 1)
	go func() { _, f := caller.call(context.Background(), in, request, "r", false); done <- f }()
	time.Sleep(30 * time.Millisecond)
	takeOverBreaker(t, cb)
	close(proceed)
	if failure := <-done; failure == nil || failure.Status != http.StatusInternalServerError {
		t.Fatalf("failure = %+v, want HTTP 500", failure)
	}
	cb.mu.Lock()
	cb.openUntil = time.Time{}
	cb.mu.Unlock()
	if _, ok := cb.reserve(time.Now()); ok {
		t.Fatal("a failed non-probe call cleared the probe of another call: second probe granted")
	}
}

// deadlineErrTransport reports a wire error (not the context error) once the
// call context is done, the way a connection may fail exactly at the deadline.
type deadlineErrTransport struct {
	body bool
	err  error
}

type deadlineErrBody struct {
	ctx context.Context
	err error
}

func (b deadlineErrBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.err }
func (deadlineErrBody) Close() error               { return nil }

func (dt deadlineErrTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if dt.body {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: deadlineErrBody{ctx: r.Context(), err: dt.err}, Request: r}, nil
	}
	<-r.Context().Done()
	return nil, dt.err
}

func TestJobDeadlineKeepsWhatHappenedOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		name string
		tr   deadlineErrTransport
		code string
	}{
		{"body cut short", deadlineErrTransport{body: true, err: io.ErrUnexpectedEOF}, "upstream_response_error"},
		{"transport error after send", deadlineErrTransport{err: errors.New("read tcp: connection reset by peer")}, "upstream_transport_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := NewCaller(Config{UpstreamTimeout: 5 * time.Second})
			caller.client.Transport = tc.tr
			in := InstanceConfig{ID: "model-a", BaseURL: "http://upstream.invalid/v1", Model: "m", Timeout: 5 * time.Second}
			request, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"complexity":"low"}`))
			if err != nil {
				t.Fatal(err)
			}
			for i := range 3 {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
				_, failure := caller.call(ctx, in, request, "r", false)
				cancel()
				if failure == nil || !failure.JobDeadline || failure.InstanceTimeout || failure.Code != tc.code || failure.BeforeSend {
					t.Fatalf("call %d failure = %+v, want JobDeadline with %s sent", i, failure, tc.code)
				}
				if !uncertainUpstreamOutcomeForInstance(in, failure) {
					t.Fatalf("call %d: the outcome must stay uncertain with abort_on_disconnect: false", i)
				}
				if status, _, code, _ := upstreamFailureResponse(failure); status != 504 || code != "request_deadline_exceeded" {
					t.Fatalf("call %d response = %d %s, want 504 request_deadline_exceeded", i, status, code)
				}
				s := tieredFixture()
				s.observeCall(schedulerJob("j", 1, false), schedulerInstance("fast"), failure, time.Second)
				if _, cooling := s.cooling["fast"]; cooling || len(s.expected) != 0 {
					t.Fatalf("call %d: observeCall learned from a call cut by the job deadline", i)
				}
			}
			if !caller.circuit("model-a").available(time.Now()) {
				t.Fatal("the job deadline opened the breaker")
			}
		})
	}
}
