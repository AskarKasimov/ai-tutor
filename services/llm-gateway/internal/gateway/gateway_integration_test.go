package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"io"
	"net"
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
	paramSeen := make(chan bool, 1)
	twoStarted := make(chan struct{})
	releaseTwo := make(chan struct{})
	twoCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		upstreamBody, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Request-ID") == stableJobID("internal", "param-test") {
			paramSeen <- bytes.Contains(upstreamBody, []byte(`"temperature":0.4`))
		}
		if r.Header.Get("X-Request-ID") == stableJobID("internal", "two-waiters") {
			close(twoStarted)
			select {
			case <-releaseTwo:
			case <-r.Context().Done():
				close(twoCanceled)
			}
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if r.Header.Get("X-Request-ID") == stableJobID("internal", "large-result") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"large-1","object":"chat.completion","created":1,"model":"server-alias","choices":[{"index":0,"message":{"role":"assistant","content":"`+strings.Repeat("x", 5<<20)+`"}}]}`)
			return
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

	firstURL, firstGRPC, firstService, stopFirst := startIntegrationGateway(t, cfg)
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
	grpcResponse := grpcCompletion(t, firstGRPC, "integration-idempotency-key")
	if !bytes.Equal(grpcResponse.CompletionJson, firstResponse.Body.Bytes()) {
		t.Fatal("HTTP and gRPC replay returned different completion bytes")
	}
	conflictHTTP := postCompletion(t, firstURL, []byte(`{"messages":[{"role":"user","content":"different"}],"complexity":"low"}`), "integration-idempotency-key")
	if conflictHTTP.Code != 409 || !bytes.Contains(conflictHTTP.Body.Bytes(), []byte("idempotency_conflict")) {
		t.Fatalf("HTTP conflict=%d %s", conflictHTTP.Code, conflictHTTP.Body.String())
	}
	conflictConn, err := grpc.NewClient(firstGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	conflictCtx, conflictCancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "integration-idempotency-key"), 5*time.Second)
	different := "different"
	_, conflictErr := pb.NewGatewayClient(conflictConn).Complete(conflictCtx, &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &different}}, Complexity: pb.Complexity_COMPLEXITY_LOW})
	conflictCancel()
	_ = conflictConn.Close()
	if status.Code(conflictErr) != codes.AlreadyExists {
		t.Fatalf("gRPC conflict=%v", conflictErr)
	}
	conn, err := grpc.NewClient(firstGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	ctxOne, cancelOne := context.WithCancel(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "two-waiters"))
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	content := "hello"
	call := func(ctx context.Context) error {
		_, err := pb.NewGatewayClient(conn).Complete(ctx, &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &content}}, Complexity: pb.Complexity_COMPLEXITY_LOW})
		return err
	}
	go func() { firstDone <- call(ctxOne) }()
	select {
	case <-twoStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("two-waiter upstream did not start")
	}
	ctxTwo, cancelTwo := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "two-waiters"), 5*time.Second)
	defer cancelTwo()
	go func() { secondDone <- call(ctxTwo) }()
	waitUntil := time.Now().Add(3 * time.Second)
	for {
		firstService.mu.Lock()
		count := firstService.activeWaiters[stableJobID("internal", "two-waiters")]
		firstService.mu.Unlock()
		if count >= 2 {
			break
		}
		if time.Now().After(waitUntil) {
			t.Fatal("second waiter did not attach")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelOne()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first waiter did not cancel")
	}
	select {
	case <-twoCanceled:
		t.Fatal("upstream canceled while a waiter remained")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseTwo)
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second waiter did not complete")
	}
	_ = conn.Close()
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("terminal-race-%d-%d", time.Now().UnixNano(), i)
		raceReq, parseErr := ParseRequest(requestBody)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		job := &durableJob{Version: 1, ID: id, ClientID: "internal", Hash: raceReq.CanonicalHash(), Request: raceReq, SubmittedAt: time.Now(), Deadline: time.Now().Add(10 * time.Second), Status: "queued"}
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := firstService.save(saveCtx, job); err != nil {
			t.Fatal(err)
		}
		saveCancel()
		firstService.mu.Lock()
		firstService.activeWaiters[id] = 1
		firstService.waiters[id] = make(chan struct{})
		firstService.mu.Unlock()
		begin := make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			<-begin
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			finished <- firstService.persistResult(ctx, job, 200, []byte(`{"id":"race"}`), "succeeded")
		}()
		go func() { <-begin; firstService.detachWaiter(id) }()
		close(begin)
		<-finished
		terminalDeadline := time.Now().Add(5 * time.Second)
		var state string
		var code string
		for {
			firstService.mu.Lock()
			current := firstService.jobs[id]
			if current != nil {
				state = current.Status
				if current.Error != nil {
					code = current.Error.Code
				}
			}
			firstService.mu.Unlock()
			if state == "succeeded" || state == "failed" {
				break
			}
			if time.Now().After(terminalDeadline) {
				t.Fatalf("race %d did not persist terminal", i)
			}
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
		firstService.mu.Lock()
		current := firstService.jobs[id]
		stable := current.Status == state && (state == "succeeded" || current.Error != nil && current.Error.Code == code)
		firstService.mu.Unlock()
		if !stable || state == "failed" && code != "request_canceled" {
			t.Fatalf("race %d unstable terminal: %s %s", i, state, code)
		}
	}
	large := grpcCompletion(t, firstGRPC, "large-result")
	if len(large.CompletionJson) <= 4<<20 {
		t.Fatalf("large completion bytes=%d", len(large.CompletionJson))
	}
	largeHTTP := postCompletion(t, firstURL, requestBody, "large-result")
	if largeHTTP.Code != 200 || !bytes.Equal(largeHTTP.Body.Bytes(), large.CompletionJson) {
		t.Fatalf("HTTP large replay code=%d bytes=%d", largeHTTP.Code, largeHTTP.Body.Len())
	}
	temp := 0.4
	paramContent := "hello"
	paramRequest := &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &paramContent}}, Complexity: pb.Complexity_COMPLEXITY_LOW, Parameters: &pb.GenerationParameters{Temperature: &temp}}
	paramConn, err := grpc.NewClient(firstGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	paramCtx, paramCancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "param-test"), 10*time.Second)
	paramResult, err := pb.NewGatewayClient(paramConn).Complete(paramCtx, paramRequest)
	paramCancel()
	_ = paramConn.Close()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case seen := <-paramSeen:
		if !seen {
			t.Fatal("upstream did not receive temperature")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parameter upstream not called")
	}
	paramHTTP := postCompletion(t, firstURL, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low","temperature":0.4}`), "param-test")
	if paramHTTP.Code != 200 || !bytes.Equal(paramHTTP.Body.Bytes(), paramResult.CompletionJson) {
		t.Fatalf("parameter HTTP replay=%d %s", paramHTTP.Code, paramHTTP.Body.String())
	}
	stopFirst()

	// A fresh request group must start at the beginning and terminally record
	// durable input that has no waiting client after restart.
	orphanRequest, err := ParseRequest(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	orphanID := stableJobID("internal", "preexisting-orphan")
	orphanInput := durableRequest{Version: 1, ID: orphanID, ClientID: "internal", IdempotencyKey: "preexisting-orphan", Hash: orphanRequest.CanonicalHash(), Request: orphanRequest, SubmittedAt: time.Now(), Deadline: time.Now().Add(10 * time.Second)}
	orphanInputRaw, _ := json.Marshal(orphanInput)
	produceCtx, produceCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := admin.ProduceSync(produceCtx, &kgo.Record{Topic: requestsTopic, Key: []byte(orphanID), Value: orphanInputRaw}).FirstErr(); err != nil {
		t.Fatal(err)
	}
	produceCancel()

	// Restore state with a fresh group and ensure the idempotent replay does not call the model again.
	cfg.ConsumerGroup += "-after-restart"
	secondURL, secondGRPC, secondService, stopSecond := startIntegrationGateway(t, cfg)
	defer stopSecond()
	orphanWaitUntil := time.Now().Add(5 * time.Second)
	for {
		secondService.mu.Lock()
		job := secondService.jobs[orphanID]
		if job != nil {
			code := ""
			if job.Error != nil {
				code = job.Error.Code
			}
			status := job.Status
			secondService.mu.Unlock()
			if status != "failed" || code != "client_disconnected" {
				t.Fatalf("preexisting request status/code = %s/%s, want failed/client_disconnected", status, code)
			}
			break
		}
		secondService.mu.Unlock()
		if time.Now().After(orphanWaitUntil) {
			t.Fatal("fresh consumer group did not consume preexisting request")
		}
		time.Sleep(10 * time.Millisecond)
	}
	secondResponse := postCompletion(t, secondURL, requestBody, "integration-idempotency-key")
	if secondResponse.Code != http.StatusOK || !bytes.Contains(secondResponse.Body.Bytes(), []byte("persisted answer")) {
		t.Fatalf("replayed completion = (%d, %s), want persisted HTTP 200 completion", secondResponse.Code, secondResponse.Body.String())
	}
	paramRestart := postCompletion(t, secondURL, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low","temperature":0.4}`), "param-test")
	if paramRestart.Code != 200 || !bytes.Equal(paramRestart.Body.Bytes(), paramResult.CompletionJson) {
		t.Fatalf("parameter restart replay=%d %s", paramRestart.Code, paramRestart.Body.String())
	}
	restartedGRPC := grpcCompletion(t, secondGRPC, "integration-idempotency-key")
	if !bytes.Equal(restartedGRPC.CompletionJson, firstResponse.Body.Bytes()) {
		t.Fatal("gRPC restart replay changed saved response")
	}
	if got := upstreamCalls.Load(); got != 4 {
		t.Fatalf("upstream calls = %d, want 4 after persisted replay", got)
	}

	expired := &durableJob{Version: 1, ID: "expired-queued", SubmittedAt: time.Now().Add(-time.Minute), Deadline: time.Now().Add(-time.Second), Offset: 100, Status: "queued", Request: Request{Messages: []Message{{Role: "user", Content: "expired"}}, Complexity: "low"}}
	secondService.install(expired)
	secondService.mu.Lock()
	secondService.inFlight["model-a"] = 1
	secondService.mu.Unlock()
	secondService.expireQueued(time.Now())
	secondService.mu.Lock()
	gotStatus := secondService.jobs[expired.ID].Status
	secondService.mu.Unlock()
	if gotStatus != "expired" {
		t.Fatalf("expired request with a full pool status = %q, want expired", gotStatus)
	}
}

func createIntegrationTopic(t *testing.T, client *kgo.Client, topic string) {
	t.Helper()
	req := kmsg.NewPtrCreateTopicsRequest()
	limit := "41943040"
	req.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: topic, NumPartitions: 1, ReplicationFactor: 1, Configs: []kmsg.CreateTopicsRequestTopicConfig{{Name: "max.message.bytes", Value: &limit}}}}
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

func startIntegrationGateway(t *testing.T, cfg Config) (string, string, *Service, func()) {
	t.Helper()
	service, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- service.Run(ctx) }()
	server := httptest.NewServer(service.Handler())
	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterGatewayServer(grpcServer, NewGRPCServer(service))
	go func() { _ = grpcServer.Serve(grpcListener) }()
	var stopOnce sync.Once
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, requestErr := http.Get(server.URL + "/health/ready")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return server.URL, grpcListener.Addr().String(), service, func() {
					stopOnce.Do(func() {
						server.Close()
						grpcServer.Stop()
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
	grpcServer.Stop()
	cancel()
	runErr := <-runDone
	service.Close()
	t.Fatalf("gateway did not become ready before timeout (Run error: %v)", runErr)
	return "", "", nil, func() {}
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

func grpcCompletion(t *testing.T, addr, key string) *pb.CompleteResponse {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(40<<20)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "idempotency-key", key)
	content := "hello"
	var header metadata.MD
	result, err := pb.NewGatewayClient(conn).Complete(ctx, &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &content}}, Complexity: pb.Complexity_COMPLEXITY_LOW}, grpc.Header(&header))
	if err != nil {
		t.Fatal(err)
	}
	if ids := header.Get("x-request-id"); len(ids) != 1 || ids[0] != result.RequestId {
		t.Fatalf("metadata request ID=%v response ID=%s", ids, result.RequestId)
	}
	return result
}

func TestGRPCCancellationStopsUpstreamAndPersistsOutcome(t *testing.T) {
	brokers := strings.Split(os.Getenv("TEST_KAFKA_BROKERS"), ",")
	if strings.TrimSpace(brokers[0]) == "" {
		t.Skip("set TEST_KAFKA_BROKERS to run Redpanda integration tests")
	}
	prefix := fmt.Sprintf("llm-gateway-cancel-%d", time.Now().UnixNano())
	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	requestsTopic, stateTopic := prefix+"-requests", prefix+"-state"
	createIntegrationTopic(t, admin, requestsTopic)
	createIntegrationTopic(t, admin, stateTopic)
	defer deleteIntegrationTopic(t, admin, requestsTopic, stateTopic)
	started := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	httpStarted := make(chan struct{})
	httpCanceled := make(chan struct{})
	grpcDeadlineStarted := make(chan struct{})
	grpcDeadlineCanceled := make(chan struct{})
	httpDeadlineStarted := make(chan struct{})
	httpDeadlineCanceled := make(chan struct{})
	stopUpstream := make(chan struct{})
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		start, canceled := started, upstreamCanceled
		switch r.Header.Get("X-Request-ID") {
		case stableJobID("internal", "cancel-http"):
			start, canceled = httpStarted, httpCanceled
		case stableJobID("internal", "deadline-grpc"):
			start, canceled = grpcDeadlineStarted, grpcDeadlineCanceled
		case stableJobID("internal", "deadline-http"):
			start, canceled = httpDeadlineStarted, httpDeadlineCanceled
		}
		close(start)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-stopUpstream:
		}
	}))
	defer func() { close(stopUpstream); upstream.CloseClientConnections(); upstream.Close() }()
	cfg := Config{KafkaBrokers: brokers, RequestsTopic: requestsTopic, StateTopic: stateTopic, ConsumerGroup: prefix + "-group", TransactionalID: prefix + "-tx", RequestDeadline: 10 * time.Second, UpstreamTimeout: 3 * time.Second, Retention: time.Hour, MaxInputLag: time.Minute, GracePeriod: time.Second, MaxBodyBytes: 1 << 20, MaxPending: 10, MaxPendingBytes: 1 << 20, MaxConnections: 10, Pools: map[string][]string{"low": {"model-a"}, "medium": {"model-a"}, "high": {"model-a"}}, Instances: []InstanceConfig{{ID: "model-a", BaseURL: upstream.URL + "/v1", Model: "served-model", MaxInFlight: 1, SupportedResponseFormats: []string{"text"}, ContextWindow: 8192, Timeout: 3 * time.Second}}}
	url, grpcAddr, cancelService, stop := startIntegrationGateway(t, cfg)
	defer stop()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "cancel-key"))
	done := make(chan error, 1)
	content := "hello"
	go func() {
		_, err := pb.NewGatewayClient(conn).Complete(ctx, &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &content}}, Complexity: pb.Complexity_COMPLEXITY_LOW})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not start")
	}
	queuedCtx, queuedCancel := context.WithCancel(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "queued-cancel"))
	queuedDone := make(chan error, 1)
	go func() {
		_, err := pb.NewGatewayClient(conn).Complete(queuedCtx, &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &content}}, Complexity: pb.Complexity_COMPLEXITY_LOW})
		queuedDone <- err
	}()
	queuedID := stableJobID("internal", "queued-cancel")
	queuedWait := time.Now().Add(5 * time.Second)
	for {
		cancelService.mu.Lock()
		queued := cancelService.jobs[queuedID] != nil && cancelService.jobs[queuedID].Status == "queued"
		cancelService.mu.Unlock()
		if queued {
			break
		}
		if time.Now().After(queuedWait) {
			t.Fatal("second request did not queue")
		}
		time.Sleep(10 * time.Millisecond)
	}
	queuedCancel()
	select {
	case <-queuedDone:
	case <-time.After(5 * time.Second):
		t.Fatal("queued cancellation did not return")
	}
	queuedReplay := postCompletion(t, url, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "queued-cancel")
	if queuedReplay.Code != 409 || !bytes.Contains(queuedReplay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("queued replay=%d %s", queuedReplay.Code, queuedReplay.Body.String())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("gRPC cancellation did not return")
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream context was not canceled")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		req, _ := http.NewRequest(http.MethodPost, url+"/v1/chat/completions", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`)))
		req.Header.Set("Idempotency-Key", "cancel-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == 409 && bytes.Contains(raw, []byte("request_canceled")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cancellation replay: status=%d body=%s", resp.StatusCode, raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
	httpCtx, httpCancel := context.WithCancel(context.Background())
	httpRequest, _ := http.NewRequestWithContext(httpCtx, http.MethodPost, url+"/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`))
	httpRequest.Header.Set("Idempotency-Key", "cancel-http")
	httpDone := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(httpRequest)
		if resp != nil {
			_ = resp.Body.Close()
		}
		httpDone <- err
	}()
	select {
	case <-httpStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP upstream did not start")
	}
	httpCancel()
	select {
	case <-httpDone:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP cancellation did not return")
	}
	select {
	case <-httpCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP upstream context was not canceled")
	}
	httpReplay := postCompletion(t, url, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "cancel-http")
	if httpReplay.Code != 409 || !bytes.Contains(httpReplay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("HTTP cancellation replay=%d %s", httpReplay.Code, httpReplay.Body.String())
	}
	grpcDeadlineCtx, grpcDeadlineCancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "idempotency-key", "deadline-grpc"), 750*time.Millisecond)
	defer grpcDeadlineCancel()
	grpcDeadlineDone := make(chan error, 1)
	go func() {
		_, err := pb.NewGatewayClient(conn).Complete(grpcDeadlineCtx, &pb.CompleteRequest{Messages: []*pb.Message{{Role: pb.Role_ROLE_USER, Content: &content}}, Complexity: pb.Complexity_COMPLEXITY_LOW})
		grpcDeadlineDone <- err
	}()
	select {
	case <-grpcDeadlineStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("gRPC deadline upstream did not start")
	}
	select {
	case err := <-grpcDeadlineDone:
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("gRPC deadline status=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gRPC deadline did not return")
	}
	select {
	case <-grpcDeadlineCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("gRPC deadline did not cancel upstream")
	}
	grpcDeadlineReplay := postCompletion(t, url, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "deadline-grpc")
	if grpcDeadlineReplay.Code != 409 || !bytes.Contains(grpcDeadlineReplay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("gRPC deadline replay=%d %s", grpcDeadlineReplay.Code, grpcDeadlineReplay.Body.String())
	}
	httpDeadlineCtx, httpDeadlineCancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer httpDeadlineCancel()
	deadlineReq, _ := http.NewRequestWithContext(httpDeadlineCtx, http.MethodPost, url+"/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`))
	deadlineReq.Header.Set("Idempotency-Key", "deadline-http")
	httpDeadlineDone := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(deadlineReq)
		if resp != nil {
			_ = resp.Body.Close()
		}
		httpDeadlineDone <- err
	}()
	select {
	case <-httpDeadlineStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP deadline upstream did not start")
	}
	select {
	case <-httpDeadlineDone:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP deadline did not return")
	}
	select {
	case <-httpDeadlineCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP deadline did not cancel upstream")
	}
	httpDeadlineReplay := postCompletion(t, url, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "deadline-http")
	if httpDeadlineReplay.Code != 409 || !bytes.Contains(httpDeadlineReplay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("HTTP deadline replay=%d %s", httpDeadlineReplay.Code, httpDeadlineReplay.Body.String())
	}
	if calls.Load() != 4 {
		t.Fatalf("upstream calls=%d, want 4", calls.Load())
	}
	circuit := cancelService.caller.circuit("model-a")
	circuit.mu.Lock()
	failures := circuit.failures
	circuit.mu.Unlock()
	if failures != 0 {
		t.Fatalf("cancellations counted as circuit failures: %d", failures)
	}
	stop()
	orphanReq, err := ParseRequest([]byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`))
	if err != nil {
		t.Fatal(err)
	}
	orphan := durableJob{Version: 1, ID: stableJobID("internal", "orphan-queued"), ClientID: "internal", IdempotencyKey: "orphan-queued", Hash: orphanReq.CanonicalHash(), Request: orphanReq, SubmittedAt: time.Now(), Deadline: time.Now().Add(10 * time.Second), Status: "queued"}
	orphanRaw, _ := json.Marshal(orphan)
	produceCtx, produceCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := admin.ProduceSync(produceCtx, &kgo.Record{Topic: stateTopic, Key: stateKey(&orphan), Value: orphanRaw}).FirstErr(); err != nil {
		t.Fatal(err)
	}
	produceCancel()
	cfg.ConsumerGroup += "-restart"
	replayURL, _, _, stopReplay := startIntegrationGateway(t, cfg)
	defer stopReplay()
	replay := postCompletion(t, replayURL, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "cancel-key")
	if replay.Code != 409 || !bytes.Contains(replay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("restart replay=%d %s", replay.Code, replay.Body.String())
	}
	httpRestartReplay := postCompletion(t, replayURL, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "cancel-http")
	if httpRestartReplay.Code != 409 || !bytes.Contains(httpRestartReplay.Body.Bytes(), []byte("request_canceled")) {
		t.Fatalf("HTTP restart replay=%d %s", httpRestartReplay.Code, httpRestartReplay.Body.String())
	}
	orphanReplay := postCompletion(t, replayURL, []byte(`{"messages":[{"role":"user","content":"hello"}],"complexity":"low"}`), "orphan-queued")
	if orphanReplay.Code != 409 || !bytes.Contains(orphanReplay.Body.Bytes(), []byte("client_disconnected")) {
		t.Fatalf("orphan replay=%d %s", orphanReplay.Code, orphanReplay.Body.String())
	}
	late := durableRequest{Version: 1, ID: stableJobID("internal", "queued-cancel"), ClientID: "internal", IdempotencyKey: "queued-cancel", Hash: orphanReq.CanonicalHash(), Request: orphanReq, SubmittedAt: time.Now(), Deadline: time.Now().Add(10 * time.Second)}
	lateRaw, _ := json.Marshal(late)
	lateCtx, lateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := admin.ProduceSync(lateCtx, &kgo.Record{Topic: requestsTopic, Key: []byte(late.ID), Value: lateRaw}).FirstErr(); err != nil {
		t.Fatal(err)
	}
	lateCancel()
	time.Sleep(750 * time.Millisecond)
	if calls.Load() != 4 {
		t.Fatalf("late input resurrected canceled job: upstream calls=%d", calls.Load())
	}
}
