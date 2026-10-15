package gateway

import (
	"context"
	"encoding/json"
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

func TestCircuitBreakerOpensAndAllowsOneHalfOpenProbe(t *testing.T) {
	var circuit Circuit
	now := time.Now()
	for range 3 {
		if !circuit.acquire(now) {
			t.Fatal("closed circuit rejected request")
		}
		circuit.release(false, now)
	}
	if circuit.acquire(now.Add(14 * time.Second)) {
		t.Fatal("open circuit allowed request before reset timeout")
	}
	probeTime := now.Add(15 * time.Second)
	if !circuit.acquire(probeTime) {
		t.Fatal("circuit did not allow half-open probe after reset timeout")
	}
	if circuit.acquire(probeTime) {
		t.Fatal("circuit allowed more than one half-open probe")
	}
	circuit.release(true, probeTime)
	if !circuit.acquire(probeTime) {
		t.Fatal("successful probe did not close circuit")
	}
}
