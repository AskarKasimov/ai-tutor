package translator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestTranslateWithHTTPServer(t *testing.T) {
	var receivedAuth string
	var receivedSystemPrompt string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		receivedAuth = r.Header.Get("Authorization")

		var req struct {
			Model          string `json:"model"`
			Messages       []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if len(req.Messages) > 0 {
			receivedSystemPrompt = req.Messages[0].Content
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]string{
						"role": "assistant",
						"content": "```json\n{\"translation\": \"## Слайд 1\\n\\nМашинное обучение — это область искусственного интеллекта.\"}\n```",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := translator.New(server.Client(), server.URL, "gpt-oss-120b", "secret-token", 2, 5*time.Second)

	sampleEnglish := "## Slide 1\n\nMachine learning is a field of artificial intelligence and computer science."
	result, err := client.Translate(context.Background(), sampleEnglish)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer secret-token" {
		t.Errorf("expected Bearer secret-token, got %q", receivedAuth)
	}
	if !strings.Contains(receivedSystemPrompt, "ПРАВИЛА ПЕРЕВОДА") {
		t.Errorf("expected translation system prompt, got %q", receivedSystemPrompt)
	}
	if !strings.Contains(result, "Машинное обучение") {
		t.Errorf("expected translated content in result, got %q", result)
	}
}

func TestTranslateHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := translator.New(server.Client(), server.URL, "test-model", "", 2, 5*time.Second)

	sampleEnglish := "## Slide 1\n\nSupervised learning maps an input to an output based on example input-output pairs."
	_, err := client.Translate(context.Background(), sampleEnglish)
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error message to mention 500, got: %v", err)
	}
}
