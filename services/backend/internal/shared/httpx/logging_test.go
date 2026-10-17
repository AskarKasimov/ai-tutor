package httpx

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func TestRequestLoggerCarriesIDsAndLogsRequestAndFailure(t *testing.T) {
	var output bytes.Buffer
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel))
	handler := RequestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if LoggerFromContext(r.Context()) == nil {
			t.Fatal("request logger missing from context")
		}
		Error(r.Context(), w, errors.New("database unavailable"))
	}))
	request := httptest.NewRequest(http.MethodGet, "/tasks?private=query", nil)
	request.Header.Set("X-Request-ID", "client-request-17")
	request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if response.Header().Get("X-Request-ID") != "client-request-17" {
		t.Fatalf("request id = %q", response.Header().Get("X-Request-ID"))
	}
	if response.Header().Get("X-Trace-ID") != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trace id = %q", response.Header().Get("X-Trace-ID"))
	}
	logs := output.String()
	for _, expected := range []string{
		`"msg":"http request"`, `"error_code":"PROCESSING_UNAVAILABLE"`,
		`"error":"database unavailable"`,
		`"request_id":"client-request-17"`, `"trace_id":"0123456789abcdef0123456789abcdef"`,
		`"status":503`,
	} {
		if !strings.Contains(logs, expected) {
			t.Errorf("logs missing %s: %s", expected, logs)
		}
	}
	if strings.Count(logs, "\n") != 1 {
		t.Fatalf("request produced duplicate log entries: %s", logs)
	}
	if strings.Contains(logs, "private=query") {
		t.Fatalf("request log contains query string: %s", logs)
	}
}

func TestRequestLoggerReplacesMalformedIDs(t *testing.T) {
	handler := RequestLogger(zap.NewNop(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("X-Request-ID", "bad id")
	request.Header.Set("X-Trace-ID", "not-a-trace")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("X-Request-ID"); len(got) != 32 || got == "bad id" {
		t.Errorf("request id = %q, want generated hex id", got)
	}
	if got := response.Header().Get("X-Trace-ID"); len(got) != 32 || got == "not-a-trace" {
		t.Errorf("trace id = %q, want generated hex id", got)
	}
}

func TestErrorLogsTheCauseBehindAFault(t *testing.T) {
	var output bytes.Buffer
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel))
	handler := RequestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Error(r.Context(), w, errors.Join(fault.New(fault.Unavailable, "MODEL_UNAVAILABLE", "Модель недоступна."), errors.New("gateway queue full")))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/answers", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "queue") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if logs := output.String(); !strings.Contains(logs, `"error_code":"MODEL_UNAVAILABLE"`) || !strings.Contains(logs, "gateway queue full") {
		t.Fatalf("logs miss the cause: %s", logs)
	}
}

func TestErrorTreatsAClosedRequestAsTheClientsChoice(t *testing.T) {
	var output bytes.Buffer
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.DebugLevel))
	ctx, cancel := context.WithCancel(context.Background())
	handler := RequestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		Error(r.Context(), w, fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Голосовой сервис недоступен."))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/audio", nil).WithContext(ctx))
	logs := output.String()
	if response.Code != StatusClientClosedRequest || !strings.Contains(logs, `"error_code":"CLIENT_CLOSED_REQUEST"`) ||
		!strings.Contains(logs, `"level":"info"`) {
		t.Fatalf("response = %d, logs = %s", response.Code, logs)
	}
}
