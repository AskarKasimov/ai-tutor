package labels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc"
)

type memoryStore struct{ values map[string]string }

func (s *memoryStore) LearningReadGraphLabels(_ context.Context, keys []string) ([]db.LearningGraphLabel, error) {
	rows := []db.LearningGraphLabel{}
	for _, key := range keys {
		if label, ok := s.values[key]; ok {
			rows = append(rows, db.LearningGraphLabel{CacheKey: key, DisplayName: label})
		}
	}
	return rows, nil
}
func (s *memoryStore) SaveGraphLabels(_ context.Context, labels []db.LearningSaveGraphLabelParams) error {
	for _, p := range labels {
		s.values[p.CacheKey] = p.DisplayName
	}
	return nil
}

type fakeGateway struct {
	pb.GatewayClient
	calls   int
	request *pb.CompleteRequest
	content string
	err     error
	respond func(context.Context, *pb.CompleteRequest) (string, error)
}

func (g *fakeGateway) Complete(ctx context.Context, r *pb.CompleteRequest, _ ...grpc.CallOption) (*pb.CompleteResponse, error) {
	g.calls++
	g.request = r
	if g.err != nil {
		return nil, g.err
	}
	content := g.content
	if g.respond != nil {
		var err error
		content, err = g.respond(ctx, r)
		if err != nil {
			return nil, err
		}
	}
	completion, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	return &pb.CompleteResponse{CompletionJson: completion}, nil
}

func manyTitles() []string {
	titles := make([]string, batchSize+1)
	for i := range titles {
		titles[i] = fmt.Sprintf("Тема %d", i)
	}
	return titles
}

func echoLabels(r *pb.CompleteRequest) string {
	var input struct {
		Titles []string `json:"titles"`
	}
	_ = json.Unmarshal([]byte(r.Messages[1].GetContent()), &input)
	output, _ := json.Marshal(map[string]any{"labels": input.Titles})
	return string(output)
}

func TestLaterBatchFailureLeavesCacheUnchangedAndRetrySavesEverything(t *testing.T) {
	store := &memoryStore{values: map[string]string{"existing": "Сохранено"}}
	gateway := &fakeGateway{}
	gateway.respond = func(_ context.Context, r *pb.CompleteRequest) (string, error) {
		if gateway.calls == 2 {
			return "", errors.New("second batch failed")
		}
		return echoLabels(r), nil
	}
	client := New(store, gateway, time.Second)
	if _, err := client.Labels(context.Background(), manyTitles()); err == nil || len(store.values) != 1 {
		t.Fatalf("failed request modified cache: %v, %v", store.values, err)
	}
	if labels, err := client.Labels(context.Background(), manyTitles()); err != nil || len(labels) != batchSize+1 || len(store.values) != batchSize+2 {
		t.Fatalf("retry did not save all batches: %v, %v", store.values, err)
	}
}

func TestQueueAndEveryBatchHaveIndependentBudgets(t *testing.T) {
	const budget = 300 * time.Millisecond
	store := &memoryStore{values: map[string]string{}}
	client := New(store, &fakeGateway{}, budget)
	gateway := client.gateway.(*fakeGateway)
	gateway.respond = func(ctx context.Context, r *pb.CompleteRequest) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < budget/2 {
			return "", errors.New("batch inherited a spent timeout")
		}
		select {
		case <-time.After(180 * time.Millisecond):
			return echoLabels(r), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	client.gate <- struct{}{}
	go func() { time.Sleep(180 * time.Millisecond); <-client.gate }()
	labels, err := client.Labels(context.Background(), manyTitles())
	if err != nil || len(labels) != batchSize+1 {
		t.Fatalf("separate phases exhausted each other's budgets: %v", err)
	}
	configured := New(store, gateway, 120*time.Second)
	if configured.timeout != 120*time.Second || configured.RequestBudget(batchSize+1) < 360*time.Second {
		t.Fatal("configured LLM timeout was truncated or HTTP budget omits queue/batches")
	}
}

func TestQueueWaitIsBoundedAndCachedNamesDoNotWait(t *testing.T) {
	store := &memoryStore{values: map[string]string{cacheKey("Есть"): "Готовая тема"}}
	gateway := &fakeGateway{}
	client := New(store, gateway, 10*time.Millisecond)
	client.gate <- struct{}{}
	defer func() { <-client.gate }()
	if _, err := client.Labels(context.Background(), []string{"Есть"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Labels(context.Background(), []string{"Нет"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queue deadline = %v", err)
	}
	if gateway.calls != 0 {
		t.Fatal("queued request called model")
	}
}

func TestLabelsReusePersistentCacheAndSendOnlyUniqueSourceNames(t *testing.T) {
	store := &memoryStore{values: map[string]string{}}
	gateway := &fakeGateway{content: `{"labels":["Нейронные сети","Градиентный спуск"]}`}
	client := New(store, gateway, time.Second)
	source := "К1: Способен применять методы построения нейронных сетей"
	titles := []string{source, "Уметь использовать градиентный спуск", source}
	labels, err := client.Labels(context.Background(), titles)
	if err != nil || labels[source] != "Нейронные сети" || len(labels) != 2 {
		t.Fatalf("labels = %v, error = %v", labels, err)
	}
	if gateway.request.GetComplexity() != pb.Complexity_COMPLEXITY_LOW {
		t.Fatal("missing low complexity")
	}
	var input struct {
		Titles []string `json:"titles"`
	}
	if err := json.Unmarshal([]byte(gateway.request.Messages[1].GetContent()), &input); err != nil || len(input.Titles) != 2 || input.Titles[0] != source {
		t.Fatalf("invalid source payload: %+v", input)
	}
	// A fresh instance (like after restart) must use the database cache too.
	_, err = New(store, gateway, time.Second).Labels(context.Background(), titles)
	if err != nil || gateway.calls != 1 {
		t.Fatalf("cache missed: calls=%d, err=%v", gateway.calls, err)
	}
	_, err = client.Labels(context.Background(), []string{source + " и архитектуры"})
	if err == nil || gateway.calls != 2 {
		t.Fatal("changed source must not reuse a stale label")
	}
}

func TestInvalidBatchNeverPollutesCache(t *testing.T) {
	for _, content := range []string{
		`{"labels":["Нейронные сети"]}`,
		`{"labels":["Нейронные сети",""]}`,
		`{"labels":["Нейронные сети","one two three four five six seven"]}`,
		`{"labels":["Нейронные сети","Первая\nвторая"]}`,
		`{"labels":["Нейронные сети","` + strings.Repeat("я", 81) + `"]}`,
		`not JSON`,
	} {
		t.Run(content, func(t *testing.T) {
			store := &memoryStore{values: map[string]string{}}
			client := New(store, &fakeGateway{content: content}, time.Second)
			_, err := client.Labels(context.Background(), []string{"Компетенция", "Составляющая"})
			if err == nil || len(store.values) != 0 {
				t.Fatalf("invalid batch cached: %v, %v", store.values, err)
			}
		})
	}
}

func TestGatewayFailureCanBeRetriedAndCancelledRequestDoesNotCallModel(t *testing.T) {
	store := &memoryStore{values: map[string]string{}}
	gateway := &fakeGateway{err: errors.New("unavailable")}
	client := New(store, gateway, time.Second)
	if _, err := client.Labels(context.Background(), []string{"Сети"}); err == nil || len(store.values) != 0 {
		t.Fatal("failed request must not be cached")
	}
	gateway.err = nil
	gateway.content = `{"labels":["Нейронные сети"]}`
	if _, err := client.Labels(context.Background(), []string{"Сети"}); err != nil {
		t.Fatal(err)
	}
	client.gate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Labels(ctx, []string{"Другое"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	<-client.gate
	if gateway.calls != 2 {
		t.Fatal("cancelled request called model")
	}
}
