// Package modelapi teaches with an OpenAI-compatible chat completions model:
// it writes theory and practice questions for an outcome and checks answers.
package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Teacher struct {
	client  *http.Client
	url     string
	model   string
	timeout time.Duration
}

func New(client *http.Client, baseURL, model string, timeout time.Duration) *Teacher {
	if client == nil {
		client = http.DefaultClient
	}
	return &Teacher{client: client, url: strings.TrimRight(baseURL, "/") + "/chat/completions", model: model, timeout: timeout}
}

const lessonPrompt = `Ты — наставник, который готовит студента к экзамену по образовательным результатам (ОР).
Тебе приходит JSON с предметом, компетенцией, ОР, таксономией, учебным содержанием и уровнем подробности.
Напиши теорию по этому ОР и один практический вопрос на закрепление.
Теория: по-русски, понятно, с примером.
Подробность: brief — 3–5 предложений, standard — 2–3 коротких абзаца, deep — 4–5 абзацев с примерами.
Вопрос: проверяет именно этот ОР, на него можно ответить устно за минуту; без вариантов ответа.
Текстовые поля theory и question оформляй в Markdown: абзацы, списки, таблицы и код по необходимости. Блоки кода помещай в тройные обратные кавычки с меткой языка сразу после открывающего разделителя (например, ` + "```python, ```javascript или ```sql" + `), обязательно отделяй пустыми строками до и после (не склеивай пояснительный текст с кавычками на одной строке), внутри кода используй синтаксис комментариев выбранного языка (например, # в Python, // в JavaScript, -- в SQL); остальные пояснения пиши вне блока. Формулы записывай символами Unicode или inline-кодом, без LaTeX-разметки. Не оборачивай весь JSON в Markdown-блок.
Верни только JSON: {"theory": "...", "question": "..."}.`

const checkPrompt = `Ты — наставник, который проверяет устный ответ студента на практический вопрос по образовательному результату (ОР).
Тебе приходит JSON с ОР, вопросом, ответом студента (расшифровка речи, возможны ошибки распознавания) и флагом last_attempt.
Реши, верен ли ответ по сути: мелкие неточности формулировок и ошибки распознавания не считаются ошибкой.
Если верно: correct=true, feedback — короткая похвала и что именно получилось (1–2 предложения), question — пустая строка.
Если неверно: correct=false, feedback — разбор именно этого ответа: что не так и как правильно, с объяснением теории (3–6 предложений).
Если неверно и last_attempt=false: question — новый практический вопрос по тому же ОР, другой, чем прежний.
Если last_attempt=true: question — пустая строка.
Пиши по-русски, обращайся к студенту на «вы». Текстовые поля feedback и question оформляй в Markdown; блоки кода помещай в тройные обратные кавычки с меткой языка сразу после открывающего разделителя (например, ` + "```python, ```javascript или ```sql" + `). Разделители блока размещай на отдельных строках, отделяй блок пустыми строками до и после; сохраняй переносы и отступы. Внутри кода используй синтаксис комментариев выбранного языка (например, # в Python, // в JavaScript, -- в SQL); остальные пояснения пиши вне блока. Формулы записывай символами Unicode или inline-кодом, без LaTeX-разметки. Не оборачивай весь JSON в Markdown-блок. Верни только JSON: {"correct": true|false, "feedback": "...", "question": "..."}.`

type outcomeData struct {
	Subject            string `json:"subject"`
	Competency         string `json:"competency"`
	Outcome            string `json:"outcome"`
	Taxonomy           string `json:"taxonomy,omitempty"`
	EducationalContent string `json:"educational_content,omitempty"`
}

func outcome(subject string, item learning.Item) outcomeData {
	return outcomeData{Subject: subject, Competency: item.CompetencyName, Outcome: item.OutcomeName, Taxonomy: item.TaxonomyCode, EducationalContent: item.EducationalContent}
}

func (t *Teacher) Lesson(ctx context.Context, input learning.LessonInput) (learning.Lesson, error) {
	payload, err := json.Marshal(struct {
		Type   string      `json:"type"`
		Detail string      `json:"detail"`
		Data   outcomeData `json:"data"`
	}{"learning_outcome", string(input.Detail), outcome(input.SubjectName, input.Item)})
	if err != nil {
		return learning.Lesson{}, err
	}
	var result struct {
		Theory   *string `json:"theory"`
		Question *string `json:"question"`
	}
	if err := t.complete(ctx, lessonPrompt, string(payload), &result); err != nil {
		return learning.Lesson{}, err
	}
	if !validText(result.Theory, 6000) || !validText(result.Question, 1500) {
		return learning.Lesson{}, invalidResponse()
	}
	return learning.Lesson{Theory: strings.TrimSpace(*result.Theory), Question: strings.TrimSpace(*result.Question)}, nil
}

func (t *Teacher) Check(ctx context.Context, input learning.CheckInput) (learning.Check, error) {
	payload, err := json.Marshal(struct {
		Type        string      `json:"type"`
		Data        outcomeData `json:"data"`
		Question    string      `json:"question"`
		Answer      string      `json:"student_answer"`
		LastAttempt bool        `json:"last_attempt"`
	}{"practice_answer", outcome(input.SubjectName, input.Item), input.Question, input.Answer, input.LastAttempt})
	if err != nil {
		return learning.Check{}, err
	}
	var result struct {
		Correct  *bool   `json:"correct"`
		Feedback *string `json:"feedback"`
		Question *string `json:"question"`
	}
	if err := t.complete(ctx, checkPrompt, string(payload), &result); err != nil {
		return learning.Check{}, err
	}
	if result.Correct == nil || !validText(result.Feedback, 4000) || result.Question == nil {
		return learning.Check{}, invalidResponse()
	}
	check := learning.Check{Correct: *result.Correct, Feedback: strings.TrimSpace(*result.Feedback)}
	if !check.Correct && !input.LastAttempt {
		if !validText(result.Question, 1500) {
			return learning.Check{}, invalidResponse()
		}
		check.Question = strings.TrimSpace(*result.Question)
	}
	return check, nil
}

func validText(value *string, limit int) bool {
	if value == nil {
		return false
	}
	text := strings.TrimSpace(*value)
	return text != "" && utf8.ValidString(text) && !strings.ContainsRune(text, 0) && utf8.RuneCountInString(text) <= limit
}

func invalidResponse() error {
	return fault.New(fault.Upstream, "INVALID_TEACHER_RESPONSE", "Наставник вернул ответ в неподдерживаемом формате.")
}

func providerError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fault.New(fault.Timeout, "TEACHER_TIMEOUT", "Наставник не успел ответить.")
	}
	return fault.New(fault.Unavailable, "TEACHER_UNAVAILABLE", "Наставник временно недоступен.")
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// complete asks the model for one JSON object and decodes it into result.
func (t *Teacher) complete(ctx context.Context, system, user string, result any) error {
	body, err := json.Marshal(struct {
		Model          string    `json:"model"`
		Messages       []message `json:"messages"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
	}{
		Model: t.model, Messages: []message{{"system", system}, {"user", user}},
		ResponseFormat: struct {
			Type string `json:"type"`
		}{"json_object"},
		Temperature: 0.4, MaxTokens: 3000,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return providerError(err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return fault.New(fault.Timeout, "TEACHER_TIMEOUT", "Наставник не успел ответить.")
	default:
		return fault.New(fault.Unavailable, "TEACHER_UNAVAILABLE", "Наставник временно недоступен.")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return providerError(err)
	}
	if len(raw) > 256*1024 {
		return invalidResponse()
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &completion) != nil || len(completion.Choices) != 1 || completion.Choices[0].Message.Content == nil {
		return invalidResponse()
	}
	content := strings.TrimSpace(*completion.Choices[0].Message.Content)
	// Some models wrap JSON in a markdown fence despite the response format.
	content = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```"), "```")
	if json.Unmarshal([]byte(strings.TrimSpace(content)), result) != nil {
		return invalidResponse()
	}
	return nil
}
