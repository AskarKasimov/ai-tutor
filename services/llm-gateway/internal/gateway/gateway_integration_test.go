package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func TestGatewayPersistsCompletionAndReplaysAfterRestart(t *testing.T) {
	brokers := strings.Split(os.Getenv("TEST_KAFKA_BROKERS"), ",")
	if len(brokers) == 0 || strings.TrimSpace(brokers[0]) == "" {
		t.Skip("set TEST_KAFKA_BROKERS to run Redpanda integration tests")
	}
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	prefix := fmt.Sprintf("llm-gateway-test-%d", time.Now().UnixNano())
	requestsTopic, stateTopic := prefix+"-requests", prefix+"-state"
	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatalf("create Kafka admin client: %v", err)
	}
	defer admin.Close()
	createIntegrationTopic(t, admin, requestsTopic)
	createIntegrationTopic(t, admin, stateTopic)
	defer deleteIntegrationTopic(t, admin, requestsTopic, stateTopic)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"completion-1","object":"chat.completion","created":1,"model":"server-alias","choices":[{"index":0,"message":{"role":"assistant","content":"persisted answer"}}]}`)
	}))
	defer upstream.Close()

	cfg := Config{
		KafkaBrokers:    brokers,
		RequestsTopic:   requestsTopic,
		StateTopic:      stateTopic,
		ConsumerGroup:   prefix + "-group",
		TransactionalID: prefix + "-tx",
		RequestDeadline: 10 * time.Second,
		UpstreamTimeout: 3 * time.Second,
		Retention:       time.Hour,
		MaxInputLag:     time.Minute,
		GracePeriod:     time.Second,
		MaxBodyBytes:    1 << 20,
		MaxPending:      10,
		MaxPendingBytes: 1 << 20,
		MaxConnections:  10,
		Pools:           map[string][]string{"low": {"model-a"}, "medium": {"model-a"}, "high": {"model-a"}},
		Instances: []InstanceConfig{{
			ID: "model-a", BaseURL: upstream.URL + "/v1", Model: "served-model", MaxInFlight: 1,
			SupportedParameters: []string{"temperature"}, SupportedResponseFormats: []string{"text"}, ContextWindow: 8192, Timeout: 3 * time.Second,
		}},
	}

	firstURL, stopFirst := startIntegrationGateway(t, cfg)
	defer stopFirst()
	requestBody := []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`)
	firstResponse := postCompletion(t, firstURL, requestBody, "integration-idempotency-key")
	if firstResponse.Code != http.StatusOK || !bytes.Contains(firstResponse.Body.Bytes(), []byte("persisted answer")) {
		stopFirst()
		t.Fatalf("first completion = (%d, %s), want persisted HTTP 200 completion", firstResponse.Code, firstResponse.Body.String())
	}
	var completion map[string]json.RawMessage
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &completion); err != nil {
		stopFirst()
		t.Fatalf("decode completion: %v", err)
	}
	var model string
	if err := json.Unmarshal(completion["model"], &model); err != nil || model != "served-model" {
		stopFirst()
		t.Fatalf("completion model = %q, want served-model (err=%v)", model, err)
	}
	stopFirst()

	// Restore state with a fresh group and ensure the idempotent replay does not call the model again.
	cfg.ConsumerGroup += "-after-restart"
	secondURL, stopSecond := startIntegrationGateway(t, cfg)
	defer stopSecond()
	secondResponse := postCompletion(t, secondURL, requestBody, "integration-idempotency-key")
	if secondResponse.Code != http.StatusOK || !bytes.Contains(secondResponse.Body.Bytes(), []byte("persisted answer")) {
		t.Fatalf("replayed completion = (%d, %s), want persisted HTTP 200 completion", secondResponse.Code, secondResponse.Body.String())
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 after persisted replay", got)
	}
}

func createIntegrationTopic(t *testing.T, client *kgo.Client, topic string) {
	t.Helper()
	req := kmsg.NewPtrCreateTopicsRequest()
	req.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := client.Request(ctx, req)
	if err != nil {
		t.Fatalf("create topic %q: %v", topic, err)
	}
	result, ok := resp.(*kmsg.CreateTopicsResponse)
	if !ok || len(result.Topics) != 1 || result.Topics[0].ErrorCode != 0 {
		t.Fatalf("create topic %q response = %#v", topic, resp)
	}
}

func deleteIntegrationTopic(t *testing.T, client *kgo.Client, topics ...string) {
	t.Helper()
	req := kmsg.NewPtrDeleteTopicsRequest()
	for _, topic := range topics {
		req.Topics = append(req.Topics, kmsg.DeleteTopicsRequestTopic{Topic: &topic})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Request(ctx, req); err != nil {
		t.Errorf("delete integration topics: %v", err)
	}
}

func startIntegrationGateway(t *testing.T, cfg Config) (string, func()) {
	t.Helper()
	service, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- service.Run(ctx) }()
	server := httptest.NewServer(service.Handler())
	var stopOnce sync.Once
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, requestErr := http.Get(server.URL + "/health/ready")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return server.URL, func() {
					stopOnce.Do(func() {
						server.Close()
						cancel()
						if err := <-runDone; err != nil {
							t.Errorf("gateway Run() error during shutdown: %v", err)
						}
						service.Close()
					})
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	server.Close()
	cancel()
	_ = <-runDone
	service.Close()
	t.Fatal("gateway did not become ready before timeout")
	return "", func() {}
}

func postCompletion(t *testing.T, serverURL string, body []byte, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	clientRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	clientRequest.Header.Set("Idempotency-Key", idempotencyKey)
	clientResponse, err := http.DefaultClient.Do(clientRequest)
	if err != nil {
		t.Fatalf("POST completion: %v", err)
	}
	defer clientResponse.Body.Close()
	response.Code = clientResponse.StatusCode
	response.Header().Set("X-Request-ID", clientResponse.Header.Get("X-Request-ID"))
	_, _ = io.Copy(response.Body, clientResponse.Body)
	return response
}
