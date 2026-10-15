package gateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

type Message struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required"`
}
type Request struct {
	Messages   []Message                  `json:"messages"`
	Complexity string                     `json:"complexity"`
	Urgent     *bool                      `json:"urgent,omitempty"`
	Model      *string                    `json:"model,omitempty"`
	Stream     *bool                      `json:"stream,omitempty"`
	Parameters map[string]json.RawMessage `json:"-"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   any    `json:"param"`
	Code    string `json:"code"`
}
type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

var allowedParameters = map[string]bool{"temperature": true, "top_p": true, "stop": true, "presence_penalty": true, "frequency_penalty": true, "seed": true, "max_tokens": true, "max_completion_tokens": true, "response_format": true, "reasoning_effort": true, "n": true}

func ParseRequest(body []byte) (Request, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return Request{}, apiErr(400, "invalid_request_error", "invalid_json", "request body must be a JSON object")
	}
	if fields == nil {
		return Request{}, apiErr(400, "invalid_request_error", "invalid_json", "request body must be a JSON object")
	}
	for key := range fields {
		if key != "messages" && key != "complexity" && key != "urgent" && key != "model" && key != "stream" && !allowedParameters[key] {
			return Request{}, apiErr(400, "invalid_request_error", "unsupported_parameter", "unsupported parameter: "+key)
		}
	}
	var req Request
	if raw, ok := fields["messages"]; !ok || json.Unmarshal(raw, &req.Messages) != nil || len(req.Messages) == 0 {
		return req, apiErr(400, "invalid_request_error", "invalid_messages", "messages must be a non-empty array")
	}
	for i, m := range req.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			return req, apiErr(400, "invalid_request_error", "invalid_messages", fmt.Sprintf("messages[%d].role is unsupported", i))
		}
		var messageFields []map[string]json.RawMessage
		if json.Unmarshal(fields["messages"], &messageFields) != nil || i >= len(messageFields) {
			return req, apiErr(400, "invalid_request_error", "invalid_messages", "messages must contain objects")
		}
		if _, ok := messageFields[i]["content"]; !ok {
			return req, apiErr(400, "invalid_request_error", "invalid_messages", fmt.Sprintf("messages[%d].content is required", i))
		}
		var content string
		if bytes.Equal(bytes.TrimSpace(messageFields[i]["content"]), []byte("null")) || json.Unmarshal(messageFields[i]["content"], &content) != nil {
			return req, apiErr(400, "invalid_request_error", "invalid_messages", fmt.Sprintf("messages[%d].content must be a string", i))
		}
		for key := range messageFields[i] {
			if key != "role" && key != "content" {
				return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "unsupported message field: "+key)
			}
		}
	}
	if raw, ok := fields["complexity"]; !ok || json.Unmarshal(raw, &req.Complexity) != nil || (req.Complexity != "low" && req.Complexity != "medium" && req.Complexity != "high") {
		return req, apiErr(400, "invalid_request_error", "invalid_complexity", "complexity must be low, medium, or high")
	}
	if raw, ok := fields["urgent"]; ok {
		if err := json.Unmarshal(raw, &req.Urgent); err != nil || req.Urgent == nil {
			return req, apiErr(400, "invalid_request_error", "invalid_urgent", "urgent must be a boolean")
		}
	}
	if _, ok := fields["model"]; ok {
		return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "model is selected by the gateway")
	}
	if raw, ok := fields["stream"]; ok {
		if err := json.Unmarshal(raw, &req.Stream); err != nil || req.Stream == nil || *req.Stream {
			return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "streaming is not supported")
		}
	}
	if raw, ok := fields["n"]; ok {
		var n int
		if json.Unmarshal(raw, &n) != nil || n != 1 {
			return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "n must be 1")
		}
		delete(fields, "n")
	}
	if raw, ok := fields["max_tokens"]; ok {
		if !positiveInt(raw) {
			return req, apiErr(400, "invalid_request_error", "invalid_parameter", "max_tokens must be a positive integer")
		}
	}
	if raw, ok := fields["max_completion_tokens"]; ok {
		if !positiveInt(raw) {
			return req, apiErr(400, "invalid_request_error", "invalid_parameter", "max_completion_tokens must be a positive integer")
		}
		if _, has := fields["max_tokens"]; has {
			return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "max_tokens and max_completion_tokens cannot be combined")
		}
	}
	for _, name := range []string{"temperature", "top_p", "presence_penalty", "frequency_penalty"} {
		if raw, ok := fields[name]; ok {
			var v float64
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &v) != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return req, apiErr(400, "invalid_request_error", "invalid_parameter", name+" must be a finite number")
			}
			if name == "temperature" && (v < 0 || v > 2) || name == "top_p" && (v < 0 || v > 1) || (name == "presence_penalty" || name == "frequency_penalty") && (v < -2 || v > 2) {
				return req, apiErr(400, "invalid_request_error", "invalid_parameter", name+" is outside the supported range")
			}
		}
	}
	if raw, ok := fields["seed"]; ok {
		var v int64
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &v) != nil {
			return req, apiErr(400, "invalid_request_error", "invalid_parameter", "seed must be an integer")
		}
	}
	if raw, ok := fields["stop"]; ok {
		var v any
		if json.Unmarshal(raw, &v) != nil {
			return req, apiErr(400, "invalid_request_error", "invalid_parameter", "stop must be a string or array of strings")
		}
		switch x := v.(type) {
		case string:
		case []any:
			if len(x) == 0 || len(x) > 4 {
				return req, apiErr(400, "invalid_request_error", "invalid_parameter", "stop must contain 1 to 4 strings")
			}
			for _, e := range x {
				if _, ok := e.(string); !ok {
					return req, apiErr(400, "invalid_request_error", "invalid_parameter", "stop must contain only strings")
				}
			}
		default:
			return req, apiErr(400, "invalid_request_error", "invalid_parameter", "stop must be a string or array of strings")
		}
	}
	if raw, ok := fields["response_format"]; ok {
		var v struct {
			Type string `json:"type"`
		}
		var keys map[string]json.RawMessage
		_ = json.Unmarshal(raw, &keys)
		for key := range keys {
			if key != "type" {
				return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "JSON Schema response formats are not supported in v1")
			}
		}
		if json.Unmarshal(raw, &v) != nil || (v.Type != "text" && v.Type != "json_object") {
			return req, apiErr(400, "invalid_request_error", "unsupported_parameter", "response_format.type must be text or json_object")
		}
	}
	if raw, ok := fields["reasoning_effort"]; ok {
		var v string
		if json.Unmarshal(raw, &v) != nil || (v != "low" && v != "medium" && v != "high") {
			return req, apiErr(400, "invalid_request_error", "invalid_parameter", "reasoning_effort must be low, medium, or high")
		}
	}
	delete(fields, "messages")
	delete(fields, "complexity")
	delete(fields, "urgent")
	delete(fields, "model")
	delete(fields, "stream")
	req.Parameters = fields
	return req, nil
}

func positiveInt(raw json.RawMessage) bool {
	var n int64
	return json.Unmarshal(raw, &n) == nil && n > 0
}
func (r Request) IsUrgent() bool { return r.Urgent != nil && *r.Urgent }
func (r Request) CanonicalHash() string {
	params := map[string]any{}
	for key, raw := range r.Parameters {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			params[key] = value
		}
	}
	for key, value := range map[string]float64{"temperature": 1, "top_p": 1, "presence_penalty": 0, "frequency_penalty": 0} {
		if _, ok := params[key]; !ok {
			params[key] = value
		}
	}
	if _, ok := params["response_format"]; !ok {
		params["response_format"] = map[string]string{"type": "text"}
	}
	b, _ := json.Marshal(struct {
		Messages   []Message      `json:"messages"`
		Complexity string         `json:"complexity"`
		Urgent     bool           `json:"urgent"`
		Parameters map[string]any `json:"parameters"`
	}{r.Messages, r.Complexity, r.IsUrgent(), params})
	var out bytes.Buffer
	_ = json.Compact(&out, b)
	h := sha256.Sum256(out.Bytes())
	return hex.EncodeToString(h[:])
}
func apiErr(status int, typ, code, message string) error {
	return statusError{status: status, body: ErrorEnvelope{Error: APIError{Message: message, Type: typ, Param: nil, Code: code}}}
}

type statusError struct {
	status int
	body   ErrorEnvelope
}

func (e statusError) Error() string { return e.body.Error.Message }
