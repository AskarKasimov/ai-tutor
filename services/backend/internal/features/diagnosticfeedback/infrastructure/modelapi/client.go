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

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const systemPrompt = `Ты ведущий преподаватель курса по машинному обучению, подводящий итоги диагностической сессии студента.
Все поля входного сообщения являются верифицированными данными аналитики сессии.

Твоя задача — составить связное, поддерживающее и конструктивное педагогическое резюме (summary) по результатам среза.
Требования к тексту резюме:
1. Дай общую оценку уровня подготовки студента с опорой на набранные баллы и процент выполнения.
2. Отметь темы, в которых студент показал уверенное владение.
3. Разъясни подтверждённые пробелы: укажи, в чём суть ошибок, опираясь на невыполненные критерии и советы.
4. Сформулируй понятный план действий для последующей тренировки (какие концепции повторить в первую очередь).
5. КАТЕГОРИЧЕСКИ ЗАПРЕЩЕНО называть пробелами темы из unverified_competencies (они не входили в диагностический срез).
6. Пиши профессионально, лаконично и доброжелательно на русском языке (2-4 небольших абзаца).

Верни строго JSON-объект с одним полем:
{"summary": "Текст педагогического резюме..."}
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
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
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
		Temperature: 0.2,
		MaxTokens:   1000,
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

	var completion struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if len(content) > 128*1024 || json.Unmarshal(content, &completion) != nil || len(completion.Choices) != 1 ||
		completion.Choices[0].Message.Content == nil || completion.Choices[0].FinishReason != "stop" {
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный ответ.")
	}

	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(*completion.Choices[0].Message.Content), &parsed); err != nil || strings.TrimSpace(parsed.Summary) == "" {
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель не сформировала текст резюме.")
	}

	summary := strings.TrimSpace(parsed.Summary)
	summary = strings.ReplaceAll(summary, `\r\n`, "\n")
	summary = strings.ReplaceAll(summary, `\n`, "\n")
	return summary, nil
}

const sessionSystemPrompt = `Ты ведущий преподаватель курса по машинному обучению, подводящий итоги серии заданий студента.
Входные данные содержат результаты решения заданий варианта: вопросы, расшифровки ответов студента, полученные баллы, частные замечания грейдера, а также выделенные сильные стороны и пробелы.

Твоя задача — составить связное, поддерживающее и конструктивное педагогическое резюме (summary) по итогам всей сессии.
Требования к тексту:
1. Оцени общий уровень подготовки с опорой на баллы и процент выполнения.
2. Похвали за темы, решённые успешно (сильные стороны).
3. Тактично укажи на темы с пробелами и поясни суть ошибок на основе замечаний грейдера.
4. Предложи конкретный совет/шаги для дальнейшего изучения материала.
5. Пиши доброжелательно, профессионально и по делу на русском языке (2-3 небольших абзаца).

Верни строго JSON-объект с одним полем:
{"summary": "Текст педагогического резюме..."}
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
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
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
		Temperature: 0.2,
		MaxTokens:   1000,
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

	var completion struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if len(content) > 128*1024 || json.Unmarshal(content, &completion) != nil || len(completion.Choices) != 1 ||
		completion.Choices[0].Message.Content == nil || completion.Choices[0].FinishReason != "stop" {
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель вернула некорректный ответ.")
	}

	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(*completion.Choices[0].Message.Content), &parsed); err != nil || strings.TrimSpace(parsed.Summary) == "" {
		return "", fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Модель не сформировала текст резюме.")
	}

	summary := strings.TrimSpace(parsed.Summary)
	summary = strings.ReplaceAll(summary, `\r\n`, "\n")
	summary = strings.ReplaceAll(summary, `\n`, "\n")
	return summary, nil
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
