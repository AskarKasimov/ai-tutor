package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validGatewayYAML = `listen_address: ":8010"
grpc_listen_address: ":8011"
kafka_brokers: ["localhost:9092"]
request_deadline: 90s
upstream_timeout: 45s
retention: 24h
max_input_lag: 24h
grace_period: 30s
pools: {low: [a], medium: [a], high: [a]}
routing_profiles: {learning-assessment: [a]}
required_routing_profiles: [learning-assessment]
instances:
  - id: a
    base_url: "http://localhost:8000/v1"
    model: served-model
    token: ""
    max_in_flight: 1
    context_window: 8192
    reasoning_effort_body:
      low: {chat_template_kwargs: {enable_thinking: false}, custom_scalar: 0.5}
model_rules:
  models:
    served-model:
      reasoning_effort_body:
        low: {chat_template_kwargs: {enable_thinking: true}, other: 2}
        medium: {chat_template_kwargs: {enable_thinking: false}, scalar: 2}
`

func writeGatewayYAML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigYAMLStrictlyParsesDurationsTokensAndModelRules(t *testing.T) {
	c, err := LoadConfig(writeGatewayYAML(t, validGatewayYAML))
	if err != nil {
		t.Fatal(err)
	}
	if c.RequestDeadline != 90*time.Second || c.Instances[0].Token != "" {
		t.Fatalf("config = %#v", c)
	}
	body := c.Instances[0].ReasoningEffortBody["low"]
	if string(body["custom_scalar"]) != "0.5" || string(body["chat_template_kwargs"]) != "{\"enable_thinking\":false}" {
		t.Fatalf("merged body = %s %s", body["custom_scalar"], body["chat_template_kwargs"])
	}
	medium := c.Instances[0].ReasoningEffortBody["medium"]
	if string(medium["scalar"]) != "2" || string(medium["chat_template_kwargs"]) != "{\"enable_thinking\":false}" {
		t.Fatalf("model rule body = %v", medium)
	}
}

func TestLoadConfigYAMLRejectsUnknownDuplicateMultipleAndBadDuration(t *testing.T) {
	cases := map[string]string{
		"unknown":        validGatewayYAML + "unknown: true\n",
		"duplicate":      strings.Replace(validGatewayYAML, "listen_address:", "listen_address: \"x\"\nlisten_address:", 1),
		"multiple":       validGatewayYAML + "---\n" + validGatewayYAML,
		"duration type":  strings.Replace(validGatewayYAML, "request_deadline: 90s", "request_deadline: 0", 1),
		"duration value": strings.Replace(validGatewayYAML, "request_deadline: 90s", "request_deadline: soon", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeGatewayYAML(t, body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadConfigYAMLRequiredProfilesIncludesStandaloneEmptyList(t *testing.T) {
	c, err := LoadConfig(writeGatewayYAML(t, strings.Replace(validGatewayYAML, "required_routing_profiles: [learning-assessment]", "required_routing_profiles: []", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.RequiredRoutingProfiles) != 0 {
		t.Fatalf("required profiles = %v", c.RequiredRoutingProfiles)
	}
	bad := strings.Replace(validGatewayYAML, "routing_profiles: {learning-assessment: [a]}", "routing_profiles: {}", 1)
	if _, err := LoadConfig(writeGatewayYAML(t, bad)); err == nil || !strings.Contains(err.Error(), "learning-assessment") {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestLoadConfigYAMLRejectsExplicitZeroLimits(t *testing.T) {
	for _, field := range []string{"max_body_bytes", "max_pending", "max_pending_bytes", "max_connections", "grpc_max_receive_bytes", "grpc_max_send_bytes", "grpc_max_concurrent_streams"} {
		t.Run(field, func(t *testing.T) {
			body := validGatewayYAML + field + ": 0\n"
			if _, err := LoadConfig(writeGatewayYAML(t, body)); err == nil {
				t.Fatal("explicit zero limit unexpectedly accepted")
			}
		})
	}
}

func TestLoadConfigYAMLDefaultsAbsentLimitsAndPreservesInstanceZeroTimeoutInheritance(t *testing.T) {
	c, err := LoadConfig(writeGatewayYAML(t, validGatewayYAML+""))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxPending != 1000 || c.MaxBodyBytes != 1<<20 || c.GRPCMaxReceiveBytes != 2<<20 || c.Instances[0].Timeout != 45*time.Second {
		t.Fatalf("defaults/inherited timeout = %#v", c)
	}
	body := strings.Replace(validGatewayYAML, "    max_in_flight: 1", "    timeout: 0s\n    max_in_flight: 1", 1)
	c, err = LoadConfig(writeGatewayYAML(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if c.Instances[0].Timeout != 45*time.Second {
		t.Fatalf("instance timeout = %s", c.Instances[0].Timeout)
	}
}
