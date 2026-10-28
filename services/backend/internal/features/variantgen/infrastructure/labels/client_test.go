package labels

import (
	"context"
	"encoding/json"
	"errors"
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
func (s *memoryStore) LearningSaveGraphLabel(_ context.Context, p db.LearningSaveGraphLabelParams) error {
	s.values[p.CacheKey] = p.DisplayName
	return nil
}

type fakeGateway struct {
	pb.GatewayClient
	calls   int
	request *pb.CompleteRequest
	content string
	err     error
}

func (g *fakeGateway) Complete(_ context.Context, r *pb.CompleteRequest, _ ...grpc.CallOption) (*pb.CompleteResponse, error) {
	g.calls++
	g.request = r
	if g.err != nil {
		return nil, g.err
	}
	completion, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": g.content}, "finish_reason": "stop"}}})
	return &pb.CompleteResponse{CompletionJson: completion}, nil
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
