package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func integrationBrokers(t *testing.T) []string {
	t.Helper()
	brokers := strings.Split(os.Getenv("TEST_KAFKA_BROKERS"), ",")
	if strings.TrimSpace(brokers[0]) == "" {
		t.Skip("set TEST_KAFKA_BROKERS to run Redpanda integration tests")
	}
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	return brokers
}

func repairIntegrationConfig(t *testing.T, brokers []string, upstreamURL string) Config {
	t.Helper()
	prefix := fmt.Sprintf("llm-gateway-repair-%d", time.Now().UnixNano())
	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	createIntegrationTopic(t, admin, prefix+"-requests")
	createIntegrationTopic(t, admin, prefix+"-state")
	t.Cleanup(func() {
		cleanup, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
		if err == nil {
			defer cleanup.Close()
			deleteIntegrationTopic(t, cleanup, prefix+"-requests", prefix+"-state")
		}
	})
	return Config{
		KafkaBrokers: brokers, RequestsTopic: prefix + "-requests", StateTopic: prefix + "-state",
		ConsumerGroup: prefix + "-group", TransactionalID: prefix + "-tx",
		RequestDeadline: 4 * time.Second, UpstreamTimeout: 3 * time.Second, Retention: time.Hour,
		MaxInputLag: time.Minute, GracePeriod: time.Second, MaxBodyBytes: 1 << 20,
		MaxPending: 10, MaxPendingBytes: 1 << 20, MaxConnections: 10,
		Pools: map[string][]string{"low": {"model-a"}, "medium": {"model-a"}, "high": {"model-a"}},
		Instances: []InstanceConfig{{
			ID: "model-a", BaseURL: upstreamURL + "/v1", Model: "served-model", MaxInFlight: 1,
			SupportedResponseFormats: []string{"text"}, ContextWindow: 8192, Timeout: 3 * time.Second,
		}},
	}
}

// A failed EndTransaction leaves the state producer inside the transaction, and
// franz-go refuses to begin another one. The write must heal the producer
// instead of failing until the process restarts.
func TestStateWriteRecoversFromAnOpenTransaction(t *testing.T) {
	brokers := integrationBrokers(t)
	cfg := repairIntegrationConfig(t, brokers, "http://127.0.0.1:1")
	s, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.stateWriter.BeginTransaction(); err != nil {
		t.Fatal(err)
	}
	if res := s.stateWriter.ProduceSync(ctx, &kgo.Record{Topic: cfg.StateTopic, Key: []byte("aborted"), Value: []byte("{}")}); res.FirstErr() != nil {
		t.Fatal(res.FirstErr())
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.stateWriter.EndTransaction(canceled, kgo.TryCommit); err == nil {
		t.Fatal("EndTransaction with a canceled context succeeded")
	}
	if err := s.stateWriter.BeginTransaction(); err == nil {
		t.Fatal("producer accepts a new transaction after the failed EndTransaction; the premise of the recovery is wrong")
	}

	job := &durableJob{Version: 1, ID: "healed", ClientID: "internal", Status: "queued", SubmittedAt: time.Now(), Deadline: time.Now().Add(time.Minute), Request: Request{Complexity: "low"}}
	saveCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if err := s.save(saveCtx, job); err != nil {
		t.Fatalf("save after the failed EndTransaction = %v, want the producer healed", err)
	}
	if err := s.save(saveCtx, job); err != nil {
		t.Fatalf("second save = %v", err)
	}

	reader, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.FetchIsolationLevel(kgo.ReadCommitted()), kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{cfg.StateTopic: {0: kgo.NewOffset().AtStart()}}), kgo.FetchMaxWait(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	seen := map[string]int{}
	readCtx, readStop := context.WithTimeout(ctx, 5*time.Second)
	defer readStop()
	for seen["healed"] < 2 && readCtx.Err() == nil {
		fetches := reader.PollFetches(readCtx)
		fetches.EachRecord(func(r *kgo.Record) { seen[string(r.Key)]++ })
	}
	if seen["healed"] != 2 || seen["aborted"] != 0 {
		t.Fatalf("committed records by key = %v, want 2 healed and no aborted", seen)
	}
}

// The state write fails after the model answered. The gateway stops assigning
// and reports not ready, then writes the outcome again once the topic accepts
// writes; a retry with the same key gets the stored outcome without a second
// upstream call. The only client left while the write failed, so the stored
// outcome is the cancellation that its departure caused; it replaces the
// unsaved answer and the answer is not written over it.
func TestGatewayWritesTheResultAgainAfterAStateFailure(t *testing.T) {
	brokers := integrationBrokers(t)
	var failWrites atomic.Bool
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if upstreamCalls.Add(1) == 1 {
			failWrites.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"stored answer"}}]}`)
	}))
	defer upstream.Close()
	cfg := repairIntegrationConfig(t, brokers, upstream.URL)
	url, _, service, stop := startIntegrationGatewayWith(t, cfg, func(s *Service) {
		s.writeStateFn = func(ctx context.Context, key, value []byte) error {
			if failWrites.Load() {
				return errors.New("state topic unavailable")
			}
			return s.transactState(ctx, key, value)
		}
	})
	defer stop()

	body := []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`)
	first := postCompletion(t, url, body, "repair-key")
	if first.Code != http.StatusServiceUnavailable || !bytes.Contains(first.Body.Bytes(), []byte("state_unavailable")) {
		t.Fatalf("response during the failure = (%d, %s), want 503 state_unavailable", first.Code, first.Body.String())
	}
	if service.Ready() {
		t.Fatal("gateway stayed ready although the result was not stored")
	}
	if during := postCompletion(t, url, body, "other-key"); during.Code != http.StatusServiceUnavailable || !bytes.Contains(during.Body.Bytes(), []byte("gateway_not_ready")) {
		t.Fatalf("new request during the failure = (%d, %s), want 503 gateway_not_ready", during.Code, during.Body.String())
	}
	time.Sleep(2500 * time.Millisecond) // a ping succeeds meanwhile, the writes still fail
	if service.Ready() {
		t.Fatal("a successful ping made the gateway ready while writes still fail")
	}

	failWrites.Store(false)
	deadline := time.Now().Add(15 * time.Second)
	for !service.Ready() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !service.Ready() {
		t.Fatal("gateway did not become ready after the writes recovered")
	}
	replay := postCompletion(t, url, body, "repair-key")
	if replay.Code != http.StatusConflict || !bytes.Contains(replay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("retry with the same key = (%d, %s), want the stored cancellation", replay.Code, replay.Body.String())
	}
	if bytes.Contains(replay.Body.Bytes(), []byte("state_unavailable")) {
		t.Fatal("the 503 given during the failure was served as the outcome")
	}
	if calls := upstreamCalls.Load(); calls != 1 {
		t.Fatalf("upstream calls = %d, want 1: the job must not run twice", calls)
	}
	fresh := postCompletion(t, url, body, "fresh-key")
	if fresh.Code != http.StatusOK {
		t.Fatalf("new request after recovery = (%d, %s), want 200", fresh.Code, fresh.Body.String())
	}
}
