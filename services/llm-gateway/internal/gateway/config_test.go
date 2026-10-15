package gateway

import (
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
	if got.ListenAddress != ":8010" || got.MaxBodyBytes != 1<<20 || got.MaxPending != 1000 {
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
