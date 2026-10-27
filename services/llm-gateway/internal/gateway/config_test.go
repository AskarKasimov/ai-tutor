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
	config, err := LoadConfig(filepath.Join("..", "..", "config", "dev.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig(dev.yaml) error = %v", err)
	}
	for _, in := range config.Instances {
		if !in.AbortOnDisconnect {
			t.Errorf("instance %q must declare its verified disconnect behavior", in.ID)
		}
		if in.ServerExecutionLimit != 0 {
			t.Errorf("instance %q has unverified server_execution_limit %s", in.ID, in.ServerExecutionLimit)
		}
	}
}

func TestLoadConfigRejectsInvalidDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.json")
	if err := os.WriteFile(path, []byte(`{"kafka_brokers":["localhost:9092"],"request_deadline":"soon"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("invalid request_deadline was accepted")
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

func TestRequiredRoutingProfilesValidatePresenceAndHighEligibility(t *testing.T) {
	c := Config{
		Pools: map[string][]string{"high": {"strong"}, "low": {"cheap"}},
		RoutingProfiles: map[string][]string{
			"optional":            {"cheap"},
			"learning-assessment": {"strong"},
			"learning-tutor":      {"strong", "cheap"},
			"learning-taskgen":    {"strong"},
		},
	}
	if err := ValidateRequiredRoutingProfiles(c, nil); err != nil {
		t.Fatalf("standalone config rejected: %v", err)
	}
	required := []string{"learning-assessment", "learning-tutor", "learning-taskgen"}
	if err := ValidateRequiredRoutingProfiles(c, required); err != nil {
		t.Fatalf("configured profiles rejected: %v", err)
	}

	if err := ValidateRequiredRoutingProfiles(Config{Pools: c.Pools, RoutingProfiles: map[string][]string{}}, required); err == nil || !strings.Contains(err.Error(), "learning-assessment") || !strings.Contains(err.Error(), "routing_profiles.learning-assessment") {
		t.Fatalf("missing profile error = %v", err)
	}
	c.RoutingProfiles["learning-assessment"] = []string{"cheap"}
	if err := ValidateRequiredRoutingProfiles(c, required); err == nil || !strings.Contains(err.Error(), "high complexity") {
		t.Fatalf("low-only profile error = %v", err)
	}
}

func TestLoadAppliesTheModelRulesFromGit(t *testing.T) {
	c, err := LoadConfig(filepath.Join("..", "..", "config", "dev.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig(dev.yaml) error = %v", err)
	}
	for _, in := range c.Instances {
		if in.ID == "qwen-3-6" {
			var kwargs struct {
				EnableThinking *bool `json:"enable_thinking"`
			}
			if err := json.Unmarshal(in.ReasoningEffortBody["low"]["chat_template_kwargs"], &kwargs); err != nil || kwargs.EnableThinking == nil || *kwargs.EnableThinking {
				t.Fatalf("qwen-3-6 low body = %v, %v", in.ReasoningEffortBody, err)
			}
		}
		if in.ID != "qwen-3-6" && len(in.ReasoningEffortBody) != 0 {
			t.Fatalf("%s got a rule of another model: %v", in.ID, in.ReasoningEffortBody)
		}
	}

}

func TestTheInstanceConfigWinsOverTheModelRule(t *testing.T) {
	c := Config{Instances: []InstanceConfig{{ID: "a", Model: "m",
		ReasoningEffortBody: map[string]map[string]json.RawMessage{"low": {"x": json.RawMessage(`"instance"`)}}}}}
	var rules ModelRules
	if err := json.Unmarshal([]byte(`{"models":{"m":{"reasoning_effort_body":{"low":{"x":"rule"},"medium":{"y":"rule"}}}}}`), &rules); err != nil {
		t.Fatal(err)
	}
	c.applyModelRules(rules)
	body := c.Instances[0].ReasoningEffortBody
	if string(body["low"]["x"]) != `"instance"` || string(body["medium"]["y"]) != `"rule"` {
		t.Fatalf("merged body = %v", body)
	}
}

func TestModelRulesGoThroughTheSameFieldCheck(t *testing.T) {
	fixture := strings.Replace(validGatewayYAML, "    reasoning_effort_body:\n      low: {chat_template_kwargs: {enable_thinking: false}, custom_scalar: 0.5}\n", "", 1)
	fixture = strings.Replace(fixture, "{chat_template_kwargs: {enable_thinking: true}, other: 2}", "{urgent: true}", 1)
	if _, err := LoadConfig(writeGatewayYAML(t, fixture)); err == nil || !strings.Contains(err.Error(), "reasoning_effort_body") {
		t.Fatalf("a rule with a gateway field was accepted: %v", err)
	}
}

func TestLoadConfigRejectsInvalidServerExecutionLimit(t *testing.T) {
	for _, limit := range []string{"soon", "-1s"} {
		t.Run(limit, func(t *testing.T) {
			body := strings.Replace(validGatewayYAML, "context_window: 8192", "context_window: 8192\n    server_execution_limit: "+limit, 1)
			if _, err := LoadConfig(writeGatewayYAML(t, body)); err == nil {
				t.Fatal("invalid server_execution_limit was accepted")
			}
		})
	}
}
