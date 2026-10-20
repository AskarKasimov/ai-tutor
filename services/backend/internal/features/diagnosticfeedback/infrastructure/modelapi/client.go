package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const systemPrompt = `Ты преподаватель курса по машинному обучению. По JSON-отчёту диагностики напиши для студента один связный абзац общего вывода (3–4 предложения).

Отчёт — единственный источник фактов:
- strengths — проверенные образовательные результаты, на которые дан полный ответ; это не доказательство освоения всей компетенции;
- confirmed_gaps — неосвоенные образовательные результаты с разбором ответа и советом;
- partial_competencies — частично выполненные основные задания; details указывает составляющую и проверенный результат.
- advice и rationale различают «дополнительно изучить» и «необходимо изучить полностью»; сохраняй эту границу, не расширяй вывод на другие составляющие.

Правила:
- Называй только темы из отчёта. Не добавляй темы, методы, формулы и примеры, которых в отчёте нет.
- Если strengths пуст, не утверждай, что что-то получилось, понято или освоено: честно скажи, что по проверенным темам пока есть пробелы, и что персональный план поможет их закрыть.
- Если confirmed_gaps и partial_competencies пусты, не придумывай пробелы.
- Не перечисляй все темы и не дублируй карточки: «Что хорошо» и «Что доработать» студент видит отдельно, собранными из разборов его ответов. Дай общую картину и главный следующий шаг.
- Не называй пробелами unverified_competencies: они не проверялись.
- Не пересказывай задания и не предлагай выбрать вариант, назвать ответ или ответить на вопрос: диагностика завершена. Следующий шаг — изучение тем из отчёта по персональному плану.
- Без заголовков, списков, баллов, процентов, приветствий и похвалы ради похвалы. Обращайся на «вы».

Поле summary оформляй в Markdown, используя выделение и inline-код по необходимости; соблюдай заданную структуру резюме. Не оборачивай весь JSON в Markdown-блок.
Верни строго JSON-объект с одним полем:
{"summary": "Абзац общего вывода"}
Не добавляй никакого текста вне JSON.`

type Client struct {
	client  *http.Client
	url     string
	model   string
	timeout time.Duration
}

func New(client *http.Client, baseURL, model string, timeout time.Duration) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{
		client:  client,
		url:     strings.TrimRight(baseURL, "/") + "/chat/completions",
		model:   model,
		timeout: timeout,
	}
}

func providerError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fault.New(fault.Timeout, "MODEL_TIMEOUT", "Превышено время ожидания генерации фидбэка.")
	}
	return fault.New(fault.Unavailable, "MODEL_UNAVAILABLE", "Сервис генерации фидбэка недоступен.")
}

func (c *Client) Synthesize(ctx context.Context, report application.DeterministicReport) (string, error) {
	data, err := json.Marshal(struct {
		DiagnosticScore         int                                 `json:"diagnostic_score"`
		MaximumScore            int                                 `json:"maximum_score"`
		ScorePercentage         int                                 `json:"score_percentage"`
		Strengths               []string                            `json:"strengths"`
		ConfirmedGaps           []applicationConfirmedGap           `json:"confirmed_gaps"`
		PartialCompetencies     []applicationPartial                `json:"partial_competencies"`
		UnverifiedCompetencies  []string                            `json:"unverified_competencies"`
		TrainingRecommendations []applicationTrainingRecommendation `json:"training_recommendations"`
	}{
		DiagnosticScore:         report.DiagnosticScore,
		MaximumScore:            report.MaximumScore,
		ScorePercentage:         report.ScorePercentage,
		Strengths:               report.Strengths,
		ConfirmedGaps:           mapGaps(report.ConfirmedGaps),
		PartialCompetencies:     mapPartials(report.PartialCompetencies),
		UnverifiedCompetencies:  mapUnverified(report.UnverifiedCompetencies),
		TrainingRecommendations: mapRecs(report.TrainingRecommendations),
	})
	if err != nil {
		return "", err
	}

	requestBody, err := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		ReasoningEffort string  `json:"reasoning_effort,omitempty"`
		Temperature     float64 `json:"temperature"`
		MaxTokens       int     `json:"max_tokens"`
	}{
		Model: c.model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(data)},
		},
		ResponseFormat: struct {
			Type string `json:"type"`
		}{Type: "json_object"},
		ReasoningEffort: "low",
		Temperature:     0.2,
		MaxTokens:       4096,
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", providerError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			return "", fault.New(fault.Unavailable, "MODEL_UNAVAILABLE", "Сервис моделей временно недоступен.")
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return "", fault.New(fault.Timeout, "MODEL_TIMEOUT", "Превышено время ожидания ответа модели.")
		default:
			return "", fault.New(fault.Upstream, "MODEL_FAILED", "Сервис моделей не принял запрос на генерацию фидбэка.")
		}
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	if err != nil {
		return "", providerError(err)
	}

	return parseCompletionSummary(content)
}

const sessionSystemPrompt = `Ты преподаватель курса по машинному обучению (ML). Твоя задача — составить итоговое резюме серии выполненных заданий (summary).

ГЛАВНОЕ ТРЕБОВАНИЕ: МАКСИМАЛЬНАЯ ПРЕДМЕТНАЯ КОНКРЕТИКА И НАСЫЩЕННОСТЬ.
КАТЕГОРИЧЕСКИ ЗАПРЕЩЕНА ВОДА («студент показал хорошие навыки», «материал освоен в целом неплохо», «продолжайте учиться»).
Во ВСЕХ трёх частях ответа ты ОБЯЗАН называть КОНКРЕТНЫЕ темы, компетенции, модели, методы, функции потерь и метрики из переданного отчёта (например: классификация, регрессия, MSE/MAE, градиентный спуск, вырожденность матрицы XᵀX, регуляризация L1/L2, переобучение, валидация). Это лишь примеры уровня конкретики: называй только темы, которые есть в переданном отчёте, и не приписывай студенту ни успехов, ни пробелов в темах, которых в отчёте нет.

Твой ответ ДОЛЖЕН содержать ВСЕ 3 обязательные части, разделённые двойным переносом строки:
1. Связный абзац общего фидбэка из 4–5 предложений: предметная педагогическая оценка выполненных заданий по конкретным разделам ML, с указанием тем с уверенными ответами и тем, где возникли трудности (без оценок, цифр и баллов).
2. Заголовок «Что хорошо:» и следом 1–2 конкретных предложения: чётко перечисли, какие именно темы, методы и понятия ML студент понял верно и где дал точные обоснования (если явных сильных сторон нет, кратко отметь готовность к практике).
3. Заголовок «Что доработать:» и следом 1–2 конкретных предложения: чётко укажи конкретные темы ML, математические и концептуальные нюансы, где студент ошибся или допустил неточность (если пробелов нет, отметь готовность к продвинутым темам).

КАТЕГОРИЧЕСКИ ЗАПРЕЩЕНО:
- Абстрактные фразы без упоминания предметной области ML и конкретных тем.
- Приветствия, похвалы, пожелания удачи и водянистые фразы.
- Повторение набранных баллов, процентов или списков заданий.
- Пересказ условий задач.

Поле summary оформляй в Markdown, используя выделение и inline-код по необходимости; соблюдай заданную структуру резюме. Не оборачивай весь JSON в Markdown-блок.
Верни строго JSON-объект с одним полем:
{"summary": "[Предметный общий фидбэк с конкретными темами в 4-5 предложений]\n\nЧто хорошо:\n[Конкретные темы и методы ML, усвоенные студентом]\n\nЧто доработать:\n[Конкретные темы и концепции ML, требующие повторения]"}
Не добавляй никакого текста вне JSON.`

func (c *Client) SynthesizeAnswers(ctx context.Context, report application.SessionFeedbackReport) (string, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return "", err
	}

	requestBody, err := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		ReasoningEffort string  `json:"reasoning_effort,omitempty"`
		Temperature     float64 `json:"temperature"`
		MaxTokens       int     `json:"max_tokens"`
	}{
		Model: c.model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: sessionSystemPrompt},
			{Role: "user", Content: string(data)},
		},
		ResponseFormat: struct {
			Type string `json:"type"`
		}{Type: "json_object"},
		ReasoningEffort: "low",
		Temperature:     0.2,
		MaxTokens:       4096,
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", providerError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			return "", fault.New(fault.Unavailable, "MODEL_UNAVAILABLE", "Сервис моделей временно недоступен.")
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return "", fault.New(fault.Timeout, "MODEL_TIMEOUT", "Превышено время ожидания ответа модели.")
		default:
			return "", fault.New(fault.Upstream, "MODEL_FAILED", "Сервис моделей не принял запрос на генерацию фидбэка.")
		}
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	if err != nil {
		return "", providerError(err)
	}

	return parseCompletionSummary(content)
}

func parseCompletionSummary(content []byte) (string, error) {
	if len(content) > 128*1024 {
		log.Printf("[FEEDBACK_MODEL] response body too large: %d bytes", len(content))
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный ответ.")
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(content, &completion); err != nil {
		log.Printf("[FEEDBACK_MODEL] failed to unmarshal response json: %v, raw=%s", err, string(content))
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный ответ.")
	}
	if len(completion.Choices) != 1 {
		log.Printf("[FEEDBACK_MODEL] unexpected choices count: len=%d", len(completion.Choices))
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный ответ.")
	}
	if completion.Choices[0].Message.Content == nil {
		log.Printf("[FEEDBACK_MODEL] choice message content is nil: finish_reason=%s", completion.Choices[0].FinishReason)
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный ответ.")
	}

	raw := strings.TrimSpace(*completion.Choices[0].Message.Content)
	summary, err := cleanAndExtractSummary(raw)
	if err != nil {
		log.Printf("[FEEDBACK_MODEL] failed to extract summary: %v, finish_reason=%s, raw=%s", err, completion.Choices[0].FinishReason, raw)
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель не сформировала текст резюме.")
	}

	return summary, nil
}

func cleanAndExtractSummary(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("empty response content")
	}

	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 && strings.HasPrefix(lines[0], "```") {
			s = strings.Join(lines[1:], "\n")
		}
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(s), &parsed); err == nil && strings.TrimSpace(parsed.Summary) != "" {
		return formatSummary(parsed.Summary), nil
	}

	if idx := strings.Index(s, `"summary"`); idx != -1 {
		rest := s[idx+len(`"summary"`):]
		if colon := strings.Index(rest, ":"); colon != -1 {
			candidate := strings.TrimSpace(rest[colon+1:])
			candidate = strings.TrimLeft(candidate, " \t\r\n\"'")
			if endQuote := strings.LastIndex(candidate, `"`); endQuote != -1 {
				candidate = candidate[:endQuote]
			} else {
				candidate = strings.TrimRight(candidate, " \t\r\n}")
			}
			candidate = strings.TrimSpace(candidate)
			if len(candidate) > 20 {
				return formatSummary(candidate), nil
			}
		}
	}

	if !strings.HasPrefix(s, "{") && len(s) > 30 {
		return formatSummary(s), nil
	}

	return "", errors.New("could not extract summary")
}

func formatSummary(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, `\r\n`, "\n")
	s = strings.ReplaceAll(s, `\n`, "\n")
	return s
}

type applicationConfirmedGap struct {
	Competency     string   `json:"competency"`
	Outcome        string   `json:"outcome"`
	FailedCriteria []string `json:"failed_criteria"`
	Advice         string   `json:"advice"`
}

type applicationPartial struct {
	Competency string `json:"competency"`
	Details    string `json:"details"`
}

type applicationTrainingRecommendation struct {
	Competency string `json:"competency"`
	Outcome    string `json:"outcome"`
	Priority   int    `json:"priority"`
	Rationale  string `json:"rationale"`
}

func mapGaps(gaps []diagnostic.ConfirmedGap) []applicationConfirmedGap {
	res := make([]applicationConfirmedGap, 0, len(gaps))
	for _, g := range gaps {
		res = append(res, applicationConfirmedGap{
			Competency:     g.CompetencyName,
			Outcome:        g.OutcomeName,
			FailedCriteria: g.FailedCriteria,
			Advice:         g.Advice,
		})
	}
	return res
}

func mapPartials(partials []diagnostic.PartialCompetency) []applicationPartial {
	res := make([]applicationPartial, 0, len(partials))
	for _, p := range partials {
		res = append(res, applicationPartial{
			Competency: p.CompetencyName,
			Details:    p.Details,
		})
	}
	return res
}

func mapUnverified(unverified []diagnostic.SkippedCompetency) []string {
	res := make([]string, 0, len(unverified))
	for _, u := range unverified {
		res = append(res, u.Name)
	}
	return res
}

func mapRecs(recs []diagnostic.TrainingRecommendation) []applicationTrainingRecommendation {
	res := make([]applicationTrainingRecommendation, 0, len(recs))
	for _, r := range recs {
		res = append(res, applicationTrainingRecommendation{
			Competency: r.CompetencyName,
			Outcome:    r.OutcomeName,
			Priority:   r.Priority,
			Rationale:  r.Rationale,
		})
	}
	return res
}
