package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const systemPrompt = `Ты — профессиональный технический переводчик учебных материалов по Computer Science, машинному обучению и искусственному интеллекту.
Твоя задача — точно и академически грамотно перевести фрагмент слайдов лекции с английского языка на русский.

ПРАВИЛА ПЕРЕВОДА:
1. Переводи только текст и пояснения.
2. Сохраняй разметку Markdown без изменений:
   - Заголовки (#, ##, ###), маркеры списков (-, *, 1.), таблицы (|...|), жирный шрифт (**...**), курсив (*...*).
   - Заголовки слайдов вида "## Slide 1" или "## Слайд 1" переводи единообразно как "## Слайд N".
3. СТРОГО ЗАПРЕЩЕНО изменять формулы LaTeX:
   - Инлайн-формулы $...$ и блочные формулы $$...$$ или \[...\] оставляй в точности как в оригинале.
   - Не переводи и не заменяй математические переменные, греческие буквы, индексы, имена функций и операторы.
4. СТРОГО ЗАПРЕЩЕНО изменять блоки кода:
   - Блоки кода ` + "```...```" + ` и инлайн-код ` + "`...`" + ` оставляй на языке программирования без изменений.
   - Имена функций, переменных, классов, библиотек и ключевые слова кода не переводить.
5. Терминология:
   - Используй устоявшуюся русскоязычную терминологию в сфере AI/ML (например: "machine learning" -> "машинное обучение", "gradient descent" -> "градиентный спуск", "reinforcement learning" -> "обучение с подкреплением", "loss function" -> "функция потерь", "overfitting" -> "переобучение", "neural network" -> "нейронная сеть", "breadth-first search" -> "поиск в ширину", "depth-first search" -> "поиск в глубину").
   - Устоявшиеся аббревиатуры и имена алгоритмов можно сохранять на латинице (например: CNN, RNN, LSTM, BERT, GPT, ResNet, ReLU, Adam, k-NN, SVM, MDP).
6. Никаких вводных слов, приветствий, пояснений или комментариев от себя.
7. Верни только валидный JSON-объект вида:
{"translation": "..."}`

// Completer abstracts chat model completions for translation.
type Completer interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// Client translates educational Markdown materials using an OpenAI-compatible API or Completer.
type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
	apiKey     string
	workers    int
	timeout    time.Duration
	completer  Completer
}

// New creates a translator client connecting directly via HTTP.
func New(httpClient *http.Client, baseURL, model, apiKey string, workers int, timeout time.Duration) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if workers <= 0 {
		workers = 4
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/chat/completions") && baseURL != "" {
		baseURL = baseURL + "/chat/completions"
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		model:      model,
		apiKey:     apiKey,
		workers:    workers,
		timeout:    timeout,
	}
}

// NewWithCompleter creates a translator client using a custom Completer.
func NewWithCompleter(completer Completer, workers int) *Client {
	if workers <= 0 {
		workers = 4
	}
	return &Client{
		completer: completer,
		workers:   workers,
	}
}

// Translate checks language and translates English markdown lectures to Russian
// with LaTeX math and code preservation, running chunk translations concurrently.
func (c *Client) Translate(ctx context.Context, markdown string) (string, error) {
	if !NeedsTranslation(markdown) {
		return markdown, nil
	}

	chunks := SplitIntoChunks(markdown, DefaultMaxCharsPerChunk)
	if len(chunks) == 0 {
		return markdown, nil
	}

	if len(chunks) == 1 {
		return c.translateChunk(ctx, chunks[0].Text)
	}

	translatedChunks := make([]Chunk, len(chunks))
	eg, egCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, c.workers)

	for i, chunk := range chunks {
		i, chunk := i, chunk
		sem <- struct{}{}
		eg.Go(func() error {
			defer func() { <-sem }()
			translatedText, err := c.translateChunk(egCtx, chunk.Text)
			if err != nil {
				return fmt.Errorf("chunk %d: %w", chunk.Index, err)
			}
			translatedChunks[i] = Chunk{
				Index: chunk.Index,
				Text:  translatedText,
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return "", err
	}

	return JoinChunks(translatedChunks), nil
}

func (c *Client) translateChunk(ctx context.Context, text string) (string, error) {
	if c.completer != nil {
		raw, err := c.completer.Complete(ctx, systemPrompt, text)
		if err != nil {
			return "", err
		}
		return parseTranslationResponse(raw)
	}

	if c.baseURL == "" {
		return "", fmt.Errorf("translator baseURL is empty")
	}

	reqBody, err := json.Marshal(map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": text},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.2,
		"max_tokens":       4096,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	reqCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("model request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("model returned HTTP %d: %s", resp.StatusCode, string(bodySnippet))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &completion); err != nil {
		return "", fmt.Errorf("decode model response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("model returned no choices")
	}

	return parseTranslationResponse(completion.Choices[0].Message.Content)
}

func parseTranslationResponse(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("empty translation response")
	}

	// 1. If wrapped in markdown code fence like ```json { "translation": "..." } ```
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			trimmed = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}

	// 2. Try JSON with key "translation" or alternatives
	var obj struct {
		Translation string `json:"translation"`
		Translated  string `json:"translated_markdown"`
		Text        string `json:"text"`
	}
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
		if obj.Translation != "" {
			return obj.Translation, nil
		}
		if obj.Translated != "" {
			return obj.Translated, nil
		}
		if obj.Text != "" {
			return obj.Text, nil
		}
	}

	// 3. Fallback: If it's not JSON (doesn't start with '{'), the model might have returned raw markdown directly
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed, nil
	}

	maxLen := len(raw)
	if maxLen > 200 {
		maxLen = 200
	}
	return "", fmt.Errorf("could not extract translation from JSON: %s", raw[:maxLen])
}
