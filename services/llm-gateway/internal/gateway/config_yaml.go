package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type yamlDuration struct {
	Value time.Duration
	Set   bool
}

func (d *yamlDuration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		return fmt.Errorf("duration must be a string with units")
	}
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return err
	}
	d.Value = v
	d.Set = true
	return nil
}

type yamlInstance struct {
	ID                       string                    `yaml:"id"`
	BaseURL                  string                    `yaml:"base_url"`
	Model                    string                    `yaml:"model"`
	Token                    string                    `yaml:"token"`
	MaxInFlight              int                       `yaml:"max_in_flight"`
	SupportedParameters      []string                  `yaml:"supported_parameters"`
	SupportedResponseFormats []string                  `yaml:"supported_response_formats"`
	ContextWindow            int                       `yaml:"context_window"`
	Timeout                  yamlDuration              `yaml:"timeout"`
	AbortOnDisconnect        bool                      `yaml:"abort_on_disconnect"`
	ServerExecutionLimit     yamlDuration              `yaml:"server_execution_limit"`
	ReasoningEffortBody      map[string]map[string]any `yaml:"reasoning_effort_body"`
}
type yamlGateway struct {
	ListenAddress            string              `yaml:"listen_address"`
	GRPCListenAddress        string              `yaml:"grpc_listen_address"`
	GRPCMaxReceiveBytes      *int                `yaml:"grpc_max_receive_bytes"`
	GRPCMaxSendBytes         *int                `yaml:"grpc_max_send_bytes"`
	GRPCMaxConcurrentStreams *int                `yaml:"grpc_max_concurrent_streams"`
	KafkaBrokers             []string            `yaml:"kafka_brokers"`
	RequestsTopic            string              `yaml:"requests_topic"`
	StateTopic               string              `yaml:"state_topic"`
	ConsumerGroup            string              `yaml:"consumer_group"`
	TransactionalID          string              `yaml:"transactional_id"`
	MaxBodyBytes             *int64              `yaml:"max_body_bytes"`
	MaxPending               *int                `yaml:"max_pending"`
	MaxPendingBytes          *int64              `yaml:"max_pending_bytes"`
	MaxConnections           *int                `yaml:"max_connections"`
	RequestDeadline          yamlDuration        `yaml:"request_deadline"`
	UpstreamTimeout          yamlDuration        `yaml:"upstream_timeout"`
	Retention                yamlDuration        `yaml:"retention"`
	MaxInputLag              yamlDuration        `yaml:"max_input_lag"`
	GracePeriod              yamlDuration        `yaml:"grace_period"`
	Pools                    map[string][]string `yaml:"pools"`
	RoutingProfiles          map[string][]string `yaml:"routing_profiles"`
	RequiredRoutingProfiles  []string            `yaml:"required_routing_profiles"`
	Instances                []yamlInstance      `yaml:"instances"`
	ModelRules               struct {
		Models map[string]struct {
			ReasoningEffortBody map[string]map[string]any `yaml:"reasoning_effort_body"`
		} `yaml:"models"`
	} `yaml:"model_rules"`
}

func rawBody(in map[string]map[string]any) (map[string]map[string]json.RawMessage, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]map[string]json.RawMessage, len(in))
	for level, fields := range in {
		out[level] = make(map[string]json.RawMessage, len(fields))
		for key, value := range fields {
			b, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			out[level][key] = b
		}
	}
	return out, nil
}
func loadYAMLConfig(path string) (Config, error) {
	var doc yamlGateway
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read gateway config %q: %w", path, err)
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err = d.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return Config{}, fmt.Errorf("gateway config is empty")
		}
		return Config{}, fmt.Errorf("decode gateway config: %w", err)
	}
	if err = validateGatewayYAML(data); err != nil {
		return Config{}, err
	}
	var extra any
	if err = d.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, fmt.Errorf("gateway config must contain exactly one YAML document")
		}
		return Config{}, fmt.Errorf("decode trailing gateway config: %w", err)
	}
	c := Config{ListenAddress: doc.ListenAddress, GRPCListenAddress: doc.GRPCListenAddress, KafkaBrokers: doc.KafkaBrokers, RequestsTopic: doc.RequestsTopic, StateTopic: doc.StateTopic, ConsumerGroup: doc.ConsumerGroup, TransactionalID: doc.TransactionalID, Pools: doc.Pools, RoutingProfiles: doc.RoutingProfiles, RequiredRoutingProfiles: doc.RequiredRoutingProfiles}
	if doc.RequestDeadline.Set {
		c.RequestDeadline = doc.RequestDeadline.Value
	}
	if doc.UpstreamTimeout.Set {
		c.UpstreamTimeout = doc.UpstreamTimeout.Value
	}
	if doc.Retention.Set {
		c.Retention = doc.Retention.Value
	}
	if doc.MaxInputLag.Set {
		c.MaxInputLag = doc.MaxInputLag.Value
	}
	if doc.GracePeriod.Set {
		c.GracePeriod = doc.GracePeriod.Value
	}
	for _, in := range doc.Instances {
		body, e := rawBody(in.ReasoningEffortBody)
		if e != nil {
			return Config{}, fmt.Errorf("instances.%s.reasoning_effort_body: %w", in.ID, e)
		}
		timeout := time.Duration(0)
		if in.Timeout.Set {
			timeout = in.Timeout.Value
		}
		limit := time.Duration(0)
		if in.ServerExecutionLimit.Set {
			limit = in.ServerExecutionLimit.Value
		}
		c.Instances = append(c.Instances, InstanceConfig{ID: in.ID, BaseURL: in.BaseURL, Model: in.Model, Token: in.Token, MaxInFlight: in.MaxInFlight, SupportedParameters: in.SupportedParameters, SupportedResponseFormats: in.SupportedResponseFormats, ContextWindow: in.ContextWindow, Timeout: timeout, AbortOnDisconnect: in.AbortOnDisconnect, ServerExecutionLimit: limit, ReasoningEffortBody: body})
	}
	rules := ModelRules{Models: make(map[string]struct {
		ReasoningEffortBody map[string]map[string]json.RawMessage `json:"reasoning_effort_body"`
	}, len(doc.ModelRules.Models))}
	for name, model := range doc.ModelRules.Models {
		body, e := rawBody(model.ReasoningEffortBody)
		if e != nil {
			return Config{}, fmt.Errorf("model_rules.models.%s.reasoning_effort_body: %w", name, e)
		}
		rules.Models[name] = struct {
			ReasoningEffortBody map[string]map[string]json.RawMessage `json:"reasoning_effort_body"`
		}{ReasoningEffortBody: body}
	}
	c.defaults()
	// Restore explicitly supplied zero durations after defaults, preserving zero for validation and documented unbounded fields.
	if doc.RequestDeadline.Set {
		c.RequestDeadline = doc.RequestDeadline.Value
	}
	if doc.UpstreamTimeout.Set {
		c.UpstreamTimeout = doc.UpstreamTimeout.Value
	}
	if doc.Retention.Set {
		c.Retention = doc.Retention.Value
	}
	if doc.MaxInputLag.Set {
		c.MaxInputLag = doc.MaxInputLag.Value
	}
	if doc.GracePeriod.Set {
		c.GracePeriod = doc.GracePeriod.Value
	}
	if doc.GRPCMaxReceiveBytes != nil {
		c.GRPCMaxReceiveBytes = *doc.GRPCMaxReceiveBytes
	}
	if doc.GRPCMaxSendBytes != nil {
		c.GRPCMaxSendBytes = *doc.GRPCMaxSendBytes
	}
	if doc.GRPCMaxConcurrentStreams != nil {
		c.GRPCMaxConcurrentStreams = *doc.GRPCMaxConcurrentStreams
	}
	if doc.MaxBodyBytes != nil {
		c.MaxBodyBytes = *doc.MaxBodyBytes
	}
	if doc.MaxPending != nil {
		c.MaxPending = *doc.MaxPending
	}
	if doc.MaxPendingBytes != nil {
		c.MaxPendingBytes = *doc.MaxPendingBytes
	}
	if doc.MaxConnections != nil {
		c.MaxConnections = *doc.MaxConnections
	}
	for i, in := range doc.Instances {
		if in.ServerExecutionLimit.Set {
			c.Instances[i].ServerExecutionLimit = in.ServerExecutionLimit.Value
		}
	}
	c.applyModelRules(rules)
	return c, c.Validate()
}

func validateGatewayYAML(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("decode gateway config: %w", err)
	}
	return validateGatewayNode(root.Content[0], reflect.TypeOf(yamlGateway{}), "config")
}

func validateGatewayNode(node *yaml.Node, typ reflect.Type, path string) error {
	pointer := false
	for typ.Kind() == reflect.Pointer {
		pointer = true
		typ = typ.Elem()
	}
	if typ == reflect.TypeOf(yamlDuration{}) {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return fmt.Errorf("%s must be a duration string", path)
		}
		if _, err := time.ParseDuration(node.Value); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	}
	if node.Tag == "!!null" {
		if pointer {
			return nil
		}
		return fmt.Errorf("%s cannot be null", path)
	}
	switch typ.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("%s must be a mapping", path)
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if field, ok := fields[key.Value]; ok {
				if err := validateGatewayNode(node.Content[i+1], field, path+"."+key.Value); err != nil {
					return err
				}
			}
		}
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a sequence", path)
		}
		for i, child := range node.Content {
			if err := validateGatewayNode(child, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return fmt.Errorf("%s must be a mapping", path)
		}
		if typ.Elem().Kind() == reflect.Interface {
			return nil
		}
		for i := 0; i < len(node.Content); i += 2 {
			if err := validateGatewayNode(node.Content[i+1], typ.Elem(), path+"."+node.Content[i].Value); err != nil {
				return err
			}
		}
	case reflect.String:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return fmt.Errorf("%s must be a string", path)
		}
	case reflect.Bool:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return fmt.Errorf("%s must be an integer", path)
		}
	case reflect.Float32, reflect.Float64:
		if node.Kind != yaml.ScalarNode || (node.Tag != "!!int" && node.Tag != "!!float") {
			return fmt.Errorf("%s must be numeric", path)
		}
	}
	return nil
}
