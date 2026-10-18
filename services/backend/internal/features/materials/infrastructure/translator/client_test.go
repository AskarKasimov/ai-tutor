package translator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	llmgatewayv1 "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/translator"
)

type mockCompleter struct {
	completeFn func(ctx context.Context, system, prompt string) (string, error)
	calls      atomic.Int32
}

func (m *mockCompleter) Complete(ctx context.Context, system, prompt string) (string, error) {
	m.calls.Add(1)
	if m.completeFn != nil {
		return m.completeFn(ctx, system, prompt)
	}
	return `{"translation": "Перевод: ` + prompt + `"}`, nil
}

func TestTranslateSkipsRussian(t *testing.T) {
	russianText := `# Лекция 1: Введение в машинное обучение

## Слайд 1

Машинное обучение изучает методы построения алгоритмов, способных обучаться на основе данных.
`
	completer := &mockCompleter{}
	client := translator.NewWithCompleter(completer, 2)

	result, err := client.Translate(context.Background(), russianText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if completer.calls.Load() != 0 {
		t.Errorf("expected 0 calls to completer for Russian text, got %d", completer.calls.Load())
	}

	if result != russianText {
		t.Errorf("expected identical text for Russian input, got %q", result)
	}
}

func TestTranslateEnglishLectureWithCompleter(t *testing.T) {
	englishLecture := `# Lecture 1: Search Algorithms

## Slide 1

Breadth-first search explores the shallowest nodes first.
It uses a FIFO queue.

## Slide 2

The heuristic function is defined as $f(n) = g(n) + h(n)$.
Consider the Python implementation:

` + "```python\n" + `def bfs(graph, start):\n    queue = [start]\n    return queue\n` + "```\n"

	completer := &mockCompleter{
		completeFn: func(ctx context.Context, system, prompt string) (string, error) {
			if !strings.Contains(system, "ПРАВИЛА ПЕРЕВОДА") {
				return "", fmt.Errorf("missing translation instructions in system prompt")
			}
			// Simulate translation keeping math and code
			resp := map[string]string{
				"translation": `# Лекция 1: Алгоритмы поиска

## Слайд 1

Поиск в ширину исследует сначала самые неглубокие узлы.
Он использует очередь FIFO.

## Слайд 2

Эвристическая функция определяется как $f(n) = g(n) + h(n)$.
Рассмотрите реализацию на Python:

` + "```python\ndef bfs(graph, start):\n    queue = [start]\n    return queue\n```",
			}
			tokens := regexp.MustCompile(`__COURSE_KEEP_[a-f0-9]+_\d+__`).FindAllString(prompt, -1)
			if len(tokens) != 2 {
				return "", fmt.Errorf("expected protected formula and code, got %v", tokens)
			}
			resp["translation"] = strings.Replace(resp["translation"], "$f(n) = g(n) + h(n)$", tokens[0], 1)
			codeStart := strings.Index(resp["translation"], "```python")
			resp["translation"] = resp["translation"][:codeStart] + tokens[1]
			data, _ := json.Marshal(resp)
			return string(data), nil
		},
	}

	client := translator.NewWithCompleter(completer, 2)
	translated, err := client.Translate(context.Background(), englishLecture)
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}

	if completer.calls.Load() == 0 {
		t.Errorf("expected completer to be called for English lecture")
	}

	expectedSubstrings := []string{
		"# Лекция 1: Алгоритмы поиска",
		"## Слайд 1",
		"Поиск в ширину",
		"## Слайд 2",
		"$f(n) = g(n) + h(n)$",
		"```python",
		"def bfs(graph, start):",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(translated, sub) {
			t.Errorf("expected translated text to contain %q, but got:\n%s", sub, translated)
		}
	}
}

type fakeGateway struct {
	llmgatewayv1.UnimplementedGatewayServer
	request *llmgatewayv1.CompleteRequest
	content string
	err     error
}

func (f *fakeGateway) Complete(_ context.Context, r *llmgatewayv1.CompleteRequest) (*llmgatewayv1.CompleteResponse, error) {
	f.request = r
	if f.err != nil {
		return nil, f.err
	}
	completion, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": f.content}, "finish_reason": "stop"}},
	})
	return &llmgatewayv1.CompleteResponse{RequestId: "req-1", CompletionJson: completion}, nil
}

func serveGateway(t *testing.T, f *fakeGateway) *translator.Gateway {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	llmgatewayv1.RegisterGatewayServer(server, f)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return translator.NewGateway(llmgatewayv1.NewGatewayClient(conn), 5*time.Second)
}

func TestTranslateThroughGateway(t *testing.T) {
	gateway := &fakeGateway{content: "```json\n{\"translation\": \"## Слайд 1\\n\\nМашинное обучение — это область искусственного интеллекта.\"}\n```"}
	client := translator.NewWithCompleter(serveGateway(t, gateway), 2)

	sampleEnglish := "## Slide 1\n\nMachine learning is a field of artificial intelligence and computer science."
	result, err := client.Translate(context.Background(), sampleEnglish)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Машинное обучение") {
		t.Errorf("expected translated content in result, got %q", result)
	}
	r := gateway.request
	if r.GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_LOW {
		t.Errorf("complexity = %v, want low", r.GetComplexity())
	}
	if len(r.GetMessages()) != 2 || r.GetMessages()[0].GetRole() != llmgatewayv1.Role_ROLE_SYSTEM ||
		!strings.Contains(r.GetMessages()[0].GetContent(), "ПРАВИЛА ПЕРЕВОДА") || r.GetMessages()[1].GetRole() != llmgatewayv1.Role_ROLE_USER {
		t.Errorf("unexpected messages: %v", r.GetMessages())
	}
	if r.GetParameters().GetResponseFormat() != llmgatewayv1.ResponseFormat_RESPONSE_FORMAT_JSON_OBJECT {
		t.Errorf("response format = %v, want json_object", r.GetParameters().GetResponseFormat())
	}
}

func TestTranslateGatewayError(t *testing.T) {
	client := translator.NewWithCompleter(serveGateway(t, &fakeGateway{err: status.Error(codes.Unavailable, "no capacity")}), 2)

	sampleEnglish := "## Slide 1\n\nSupervised learning maps an input to an output based on example input-output pairs."
	_, err := client.Translate(context.Background(), sampleEnglish)
	if err == nil {
		t.Fatal("expected error when the gateway fails, got nil")
	}
	if !strings.Contains(err.Error(), "no capacity") {
		t.Errorf("expected the gateway error, got: %v", err)
	}
}
