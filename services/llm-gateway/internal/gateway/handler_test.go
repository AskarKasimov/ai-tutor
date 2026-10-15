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
