package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseRequestAcceptsSupportedCompletion(t *testing.T) {
	got, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"medium","temperature":0.4,"response_format":{"type":"json_object"}}`))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if got.Complexity != "medium" || len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Fatalf("ParseRequest() = %#v", got)
	}
	if got.IsUrgent() {
		t.Fatal("urgent should default to false")
	}
	if _, ok := got.Parameters["temperature"]; !ok {
		t.Fatal("temperature was not preserved")
	}
}

func TestGeneratedOpenAPIRequestDocumentsRoutingProfile(t *testing.T) {
	openapi, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(openapi)
	const schema = "    gateway.OpenAPIRequest:\n"
	start := strings.Index(contents, schema)
	if start < 0 {
		t.Fatalf("generated OpenAPI is missing schema %q", strings.TrimSpace(schema))
	}
	requestSchema := contents[start+len(schema):]
	end := len(requestSchema)
	for _, boundary := range []string{"\n    gateway.", "\ninfo:"} {
		if index := strings.Index(requestSchema, boundary); index >= 0 && index < end {
			end = index
		}
	}
	requestSchema = requestSchema[:end]
	if !strings.Contains(requestSchema, "\n        routing_profile:\n          type: string") {
		t.Fatal("generated OpenAPI request schema does not document routing_profile")
	}
}

func TestParseRequestRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "invalid JSON", body: `{`, code: "invalid_json"},
		{name: "non-object", body: `[]`, code: "invalid_json"},
		{name: "missing messages", body: `{"complexity":"low"}`, code: "invalid_messages"},
		{name: "invalid complexity", body: `{"messages":[{"role":"user","content":"x"}],"complexity":"extreme"}`, code: "invalid_complexity"},
		{name: "client model", body: `{"messages":[{"role":"user","content":"x"}],"complexity":"low","model":"anything"}`, code: "unsupported_parameter"},
		{name: "streaming", body: `{"messages":[{"role":"user","content":"x"}],"complexity":"low","stream":true}`, code: "unsupported_parameter"},
		{name: "unknown parameter", body: `{"messages":[{"role":"user","content":"x"}],"complexity":"low","tools":[]}`, code: "unsupported_parameter"},
		{name: "null content", body: `{"messages":[{"role":"user","content":null}],"complexity":"low"}`, code: "invalid_messages"},
		{name: "max token conflict", body: `{"messages":[{"role":"user","content":"x"}],"complexity":"low","max_tokens":10,"max_completion_tokens":10}`, code: "unsupported_parameter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRequest([]byte(tt.body))
			if err == nil {
				t.Fatal("ParseRequest() error = nil, want error")
			}
			var apiErr statusError
			if !errors.As(err, &apiErr) {
				t.Fatalf("ParseRequest() error type = %T, want statusError", err)
			}
			if apiErr.body.Error.Code != tt.code {
				t.Fatalf("error code = %q, want %q", apiErr.body.Error.Code, tt.code)
			}
		})
	}
}

func TestCanonicalHashNormalizesOmittedDefaults(t *testing.T) {
	withoutDefaults, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	withDefaults, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low","temperature":1,"top_p":1,"presence_penalty":0,"frequency_penalty":0,"response_format":{"type":"text"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if withoutDefaults.CanonicalHash() != withDefaults.CanonicalHash() {
		t.Fatal("equivalent requests have different canonical hashes")
	}
}

func TestCanonicalHashRestoredLegacyStop(t *testing.T) {
	legacy := Request{Messages: []Message{{Role: "user", Content: "x"}}, Complexity: "low", Parameters: map[string]json.RawMessage{"stop": json.RawMessage(`"END"`)}}
	current, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"x"}],"complexity":"low","stop":["END"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if legacy.CanonicalHash() != current.CanonicalHash() {
		t.Fatal("restored string stop differs from canonical array")
	}
}

func TestCanonicalHashKeepsLargeIntegerPrecision(t *testing.T) {
	first, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"x"}],"complexity":"low","seed":9007199254740992}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"x"}],"complexity":"low","seed":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.CanonicalHash() == second.CanonicalHash() {
		t.Fatal("distinct int64 seeds collided")
	}
}

func TestRequestDurableJSONPreservesParameters(t *testing.T) {
	original, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"x"}],"complexity":"low","temperature":0.4,"seed":9007199254740993,"stop":"END"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored Request
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.CanonicalHash() != original.CanonicalHash() {
		t.Fatalf("durable round trip changed request: %s", raw)
	}
	for _, key := range []string{"temperature", "seed", "stop"} {
		if _, ok := restored.Parameters[key]; !ok {
			t.Fatalf("parameter %s missing after round trip", key)
		}
	}
}

func TestLegacyHashReplayWithLostParameters(t *testing.T) {
	for _, stop := range []string{`"END"`, `["END"]`} {
		t.Run(stop, func(t *testing.T) {
			cfg := Config{KafkaBrokers: []string{"127.0.0.1:1"}, MaxConnections: 1, Pools: map[string][]string{"low": {"a"}}, Instances: []InstanceConfig{{ID: "a", Model: "a", MaxInFlight: 1, ContextWindow: 8192, SupportedParameters: []string{"stop"}, SupportedResponseFormats: []string{"text"}}}}
			s, err := NewService(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			req, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"x"}],"complexity":"low","stop":["END"]}`))
			if err != nil {
				t.Fatal(err)
			}
			legacy := Request{Messages: req.Messages, Complexity: req.Complexity, Parameters: map[string]json.RawMessage{"stop": json.RawMessage(stop)}}
			id := stableJobID("internal", "old-key")
			s.mu.Lock()
			s.ready = true
			s.jobs[id] = &durableJob{ID: id, ClientID: "internal", IdempotencyKey: "old-key", Hash: legacy.legacyHash(false), Request: Request{Messages: req.Messages, Complexity: req.Complexity}, Status: "succeeded", Response: json.RawMessage(`{"id":"saved"}`), SubmittedAt: time.Now()}
			s.keys[identity("internal", "old-key")] = id
			s.mu.Unlock()
			result, gotID, err := s.Complete(context.Background(), req, "old-key", "grpc", false, nil)
			if err != nil || gotID != id || result.status != 200 {
				t.Fatalf("legacy replay: id=%s result=%+v err=%v", gotID, result, err)
			}
		})
	}
}
