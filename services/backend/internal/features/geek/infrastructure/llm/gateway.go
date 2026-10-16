package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type complexity string

const (
	low    complexity = "low"
	medium complexity = "medium"
	high   complexity = "high"
)

// Gateway calls the LLM gateway: an OpenAI-compatible chat completion without
// a model field, with the required complexity.
type Gateway struct {
	client  *http.Client
	url     string
	timeout time.Duration
}

func NewGateway(client *http.Client, baseURL string, timeout time.Duration) *Gateway {
	return &Gateway{client: client, url: strings.TrimRight(baseURL, "/") + "/v1/chat/completions", timeout: timeout}
}

func (g *Gateway) Question(ctx context.Context, r application.QuestionRequest) (string, error) {
	level := medium
	if r.Kind == exam.KindRephrase {
		level = low
	}
	var out struct {
		Text string `json:"text"`
	}
	err := g.complete(ctx, "question:"+string(r.Kind), level, questionPrompt(r), &out, func() error {
		if strings.TrimSpace(out.Text) == "" {
			return errors.New("empty text")
		}
		return nil
	})
	return strings.TrimSpace(out.Text), err
}

func (g *Gateway) Evaluate(ctx context.Context, r application.EvaluationRequest) (exam.Evaluation, error) {
	var out struct {
		Verdict  exam.Verdict `json:"verdict"`
		Criteria *struct {
			K1 *int `json:"k1"`
			K2 *int `json:"k2"`
			K3 *int `json:"k3"`
			K4 *int `json:"k4"`
		} `json:"criteria"`
		Evidence []int `json:"evidence_lines"`
	}
	var c exam.Criteria
	err := g.complete(ctx, "evaluate:"+string(r.Kind), high, evaluationPrompt(r), &out, func() error {
		switch out.Verdict {
		case exam.VerdictAnswered, exam.VerdictDontKnow, exam.VerdictInsufficient:
		default:
			return fmt.Errorf("unknown verdict %q", out.Verdict)
		}
		if out.Verdict == exam.VerdictAnswered {
			k := out.Criteria
			if k == nil || k.K1 == nil || k.K2 == nil || k.K3 == nil || k.K4 == nil {
				return errors.New("missing criteria")
			}
			c = exam.Criteria{K1: *k.K1, K2: *k.K2, K3: *k.K3, K4: *k.K4}
			if !c.Valid() {
				return fmt.Errorf("criteria out of range: %+v", c)
			}
		}
		return nil
	})
	if err != nil {
		return exam.Evaluation{}, err
	}
	return exam.Evaluation{Verdict: out.Verdict, Criteria: c, Evidence: out.Evidence}, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Messages       []message         `json:"messages"`
	Complexity     complexity        `json:"complexity"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format"`
}

// complete asks for a JSON object, decodes it into out and checks it; an
// invalid answer is retried once.
func (g *Gateway) complete(ctx context.Context, operation string, level complexity, prompt string, out any, check func() error) error {
	logger := httpx.LoggerFromContext(ctx).With(zap.String("llm_operation", operation), zap.String("complexity", string(level)))
	body, err := json.Marshal(request{
		Messages:       []message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: prompt}},
		Complexity:     level,
		Temperature:    0.2,
		ResponseFormat: map[string]string{"type": "json_object"},
	})
	if err != nil {
		return err
	}
	var last error
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		content, err := g.call(ctx, body)
		logger.Info("llm call", zap.Int("attempt", attempt), zap.Duration("duration", time.Since(started)),
			zap.String("prompt", prompt), zap.String("response", content), zap.Error(err))
		if err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(stripFence(content)), out); err == nil {
			err = check()
		}
		if err == nil {
			return nil
		}
		last = fmt.Errorf("invalid model answer: %w", err)
	}
	return last
}

func (g *Gateway) call(ctx context.Context, body []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call LLM gateway: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("read LLM gateway response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return string(data), fmt.Errorf("LLM gateway status %d", resp.StatusCode)
	}
	var completion struct {
		Choices []struct {
			Message      message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &completion); err != nil || len(completion.Choices) == 0 {
		return string(data), errors.New("LLM gateway returned no choices")
	}
	if completion.Choices[0].FinishReason == "length" {
		return completion.Choices[0].Message.Content, errors.New("LLM answer was cut by the token limit")
	}
	return completion.Choices[0].Message.Content, nil
}

// stripFence removes a Markdown code fence some models put around JSON.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}
