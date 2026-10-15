package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompletionEndpointIsInternalWithoutBearerAuthentication(t *testing.T) {
	s := &Service{
		cfg:         Config{MaxConnections: 1, MaxBodyBytes: 1 << 20},
		connections: make(chan struct{}, 1),
		ready:       true,
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (authentication is not required)", response.Code)
	}
	var body ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "invalid_messages" {
		t.Fatalf("error code = %q, want invalid_messages", body.Error.Code)
	}
}

func TestVSCodePreviewPreflightAllowsCompletionRequest(t *testing.T) {
	s := &Service{cfg: Config{MaxConnections: 1, MaxBodyBytes: 1 << 20}, connections: make(chan struct{}, 1), ready: true}
	request := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	request.Header.Set("Origin", "vscode-webview://test-extension")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type,idempotency-key")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "vscode-webview://test-extension" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "idempotency-key") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want Idempotency-Key", got)
	}
}

func TestVSCodePreviewDoesNotAllowUntrustedOrigin(t *testing.T) {
	s := &Service{cfg: Config{MaxConnections: 1, MaxBodyBytes: 1 << 20}, connections: make(chan struct{}, 1), ready: true}
	request := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)

	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted origin was allowed: %q", response.Header().Get("Access-Control-Allow-Origin"))
	}
}
