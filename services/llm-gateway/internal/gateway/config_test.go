package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigParsesDurationsAndAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.json")
	config := `{"kafka_brokers":["localhost:9092"],"request_deadline":"90s","upstream_timeout":"45s","pools":{"low":["model-a"],"medium":["model-a"],"high":["model-a"]},"instances":[{"id":"model-a","base_url":"http://localhost:8000/v1","model":"served-model","max_in_flight":1,"context_window":8192}]}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got.RequestDeadline != 90*time.Second || got.UpstreamTimeout != 45*time.Second {
		t.Fatalf("durations = (%s, %s), want (90s, 45s)", got.RequestDeadline, got.UpstreamTimeout)
	}
	if got.Instances[0].Timeout != 45*time.Second {
		t.Fatalf("instance timeout = %s, want inherited 45s", got.Instances[0].Timeout)
	}
	if got.ListenAddress != ":8010" || got.MaxBodyBytes != 1<<20 || got.MaxPending != 1000 || got.GRPCListenAddress != ":8011" || got.GRPCMaxReceiveBytes != 2<<20 || got.GRPCMaxSendBytes != 40<<20 || got.GRPCMaxConcurrentStreams != 128 {
		t.Fatalf("defaults were not applied: %#v", got)
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	if _, err := LoadConfig(filepath.Join("..", "..", "config.example.json")); err != nil {
		t.Fatalf("LoadConfig(config.example.json) error = %v", err)
	}
}

func TestLoadConfigRejectsInvalidDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.json")
	if err := os.WriteFile(path, []byte(`{"kafka_brokers":["localhost:9092"],"request_deadline":"soon"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "request_deadline") {
		t.Fatalf("LoadConfig() error = %v, want request_deadline parse error", err)
	}
}

func TestConfigValidationRejectsUnknownPoolInstance(t *testing.T) {
	c := Config{
		KafkaBrokers: []string{"localhost:9092"},
		Pools:        map[string][]string{"low": {"model-a"}, "medium": {"missing"}, "high": {"model-a"}},
		Instances:    []InstanceConfig{{ID: "model-a", BaseURL: "http://localhost:8000/v1", Model: "served-model", MaxInFlight: 1, ContextWindow: 8192}},
	}
	c.defaults()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), `pools.medium references unknown instance "missing"`) {
		t.Fatalf("Validate() error = %v, want unknown pool instance", err)
	}
}

func TestConfigValidationGuardsTheReasoningEffortBody(t *testing.T) {
	valid := func(body map[string]map[string]json.RawMessage) Config {
		c := Config{
			KafkaBrokers: []string{"localhost:9092"},
			Pools:        map[string][]string{"low": {"model-a"}, "medium": {"model-a"}, "high": {"model-a"}},
			Instances: []InstanceConfig{{ID: "model-a", BaseURL: "http://localhost:8000/v1", Model: "served-model", MaxInFlight: 1, ContextWindow: 8192,
				ReasoningEffortBody: body}},
		}
		c.defaults()
		return c
	}
	ok := valid(map[string]map[string]json.RawMessage{"low": {"chat_template_kwargs": json.RawMessage(`{"enable_thinking":false}`)}})
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid body rejected: %v", err)
	}
	for name, body := range map[string]map[string]map[string]json.RawMessage{
		"unknown level":      {"extreme": {"x": json.RawMessage(`1`)}},
		"model":              {"low": {"model": json.RawMessage(`"other"`)}},
		"messages":           {"low": {"messages": json.RawMessage(`[]`)}},
		"stream":             {"low": {"stream": json.RawMessage(`true`)}},
		"a client parameter": {"low": {"temperature": json.RawMessage(`0`)}},
		"the effort itself":  {"low": {"reasoning_effort": json.RawMessage(`"high"`)}},
		"complexity":         {"low": {"complexity": json.RawMessage(`"high"`)}},
		"urgent":             {"low": {"urgent": json.RawMessage(`true`)}},
	} {
		c := valid(body)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "reasoning_effort_body") {
			t.Fatalf("%s: Validate() error = %v", name, err)
		}
	}
}
