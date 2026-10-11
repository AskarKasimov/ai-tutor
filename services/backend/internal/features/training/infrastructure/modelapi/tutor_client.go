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

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type TutorClient struct {
	client  *http.Client
	url     string
	model   string
	timeout time.Duration
}

func NewTutor(client *http.Client, baseURL, model string, timeout time.Duration) *TutorClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &TutorClient{client: client, url: strings.TrimRight(baseURL, "/") + "/chat/completions", model: model, timeout: timeout}
}

func tutorProviderError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fault.New(fault.Timeout, "TUTOR_TIMEOUT", "Репетитор не успел ответить.")
	}
	return fault.New(fault.Unavailable, "TUTOR_UNAVAILABLE", "Репетитор временно недоступен.")
}

func invalidTutorResponse() error {
	return fault.New(fault.Upstream, "INVALID_TUTOR_RESPONSE", "Репетитор вернул ответ в неподдерживаемом формате.")
}

type tutorTaskSnapshot struct {
	Question           string   `json:"question"`
	Options            []string `json:"options,omitempty"`
	VoiceInstruction   string   `json:"voice_instruction"`
	OutcomeName        string   `json:"outcome_name"`
	TaxonomyCode       string   `json:"taxonomy_code,omitempty"`
	ALDLevelCode       string   `json:"ald_level_code,omitempty"`
	EducationalContent string   `json:"educational_content,omitempty"`
	ReferenceAnswer    string   `json:"reference_answer"`
	Criteria           string   `json:"criteria,omitempty"`
}

type tutorCriterionState struct {
	Key       string `json:"key"`
	Satisfied bool   `json:"satisfied"`
}

type tutorCheckContext struct {
	Score    int                   `json:"score"`
	MaxScore int                   `json:"max_score"`
	Verdict  string                `json:"verdict"`
	Criteria []tutorCriterionState `json:"criteria"`
}

type tutorSnapshotEnvelope struct {
	Type          string             `json:"type"`
	PromptVersion string             `json:"prompt_version"`
	Task          tutorTaskSnapshot  `json:"task"`
	Check         *tutorCheckContext `json:"trusted_check,omitempty"`
}

func tutorSnapshot(ctx training.TutorContext) (tutorSnapshotEnvelope, error) {
	task := ctx.Exercise.Task
	envelope := tutorSnapshotEnvelope{
		Type: "trusted_training_exercise_data", PromptVersion: TutorPromptVersion,
		Task: tutorTaskSnapshot{Question: task.Question, Options: append([]string(nil), task.Options...), VoiceInstruction: task.VoiceInstruction, OutcomeName: task.OutcomeName, TaxonomyCode: task.TaxonomyCode, ALDLevelCode: task.ALDLevelCode, EducationalContent: task.EducationalContent, ReferenceAnswer: task.ReferenceAnswer, Criteria: task.Criteria},
	}
	if ctx.Check != nil {
		criteria := make([]tutorCriterionState, 0, len(ctx.Check.CriterionResults))
		for _, result := range ctx.Check.CriterionResults {
			criteria = append(criteria, tutorCriterionState{Key: result.Key, Satisfied: result.Satisfied})
		}
		envelope.Check = &tutorCheckContext{Score: ctx.Check.Score, MaxScore: ctx.Check.MaxScore, Verdict: ctx.Check.Verdict, Criteria: criteria}
	}
	return envelope, nil
}

type tutorRequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *TutorClient) Reply(ctx context.Context, tutorContext training.TutorContext) (training.TutorReply, error) {
	envelope, err := tutorSnapshot(tutorContext)
	if err != nil {
		return training.TutorReply{}, err
	}
	snapshotJSON, err := json.Marshal(envelope)
	if err != nil {
		return training.TutorReply{}, err
	}
	messages := []tutorRequestMessage{
		{Role: "system", Content: tutorSystemPrompt},
		{Role: "user", Content: string(snapshotJSON)},
	}
	if tutorContext.Summary != "" {
		summaryJSON, _ := json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "student_history_excerpts_data", Text: tutorContext.Summary})
		messages = append(messages, tutorRequestMessage{Role: "user", Content: string(summaryJSON)})
	}
	for _, item := range tutorContext.Recent {
		if item.Role != training.ChatRoleUser && item.Role != training.ChatRoleAssistant {
			continue
		}
		messages = append(messages, tutorRequestMessage{Role: string(item.Role), Content: item.Text})
	}
	messages = append(messages, tutorRequestMessage{Role: "user", Content: tutorContext.StudentText})
	requestBody, err := json.Marshal(struct {
		Model          string                `json:"model"`
		Messages       []tutorRequestMessage `json:"messages"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
	}{
		Model: c.model, Messages: messages,
		ResponseFormat: struct {
			Type string `json:"type"`
		}{Type: "json_object"},
		Temperature: 0.3, MaxTokens: 2048,
	})
	if err != nil {
		return training.TutorReply{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(requestBody))
	if err != nil {
		return training.TutorReply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return training.TutorReply{}, tutorProviderError(err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return training.TutorReply{}, fault.New(fault.Timeout, "TUTOR_TIMEOUT", "Репетитор не успел ответить.")
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return training.TutorReply{}, fault.New(fault.Unavailable, "TUTOR_UNAVAILABLE", "Репетитор временно недоступен.")
	default:
		return training.TutorReply{}, fault.New(fault.Unavailable, "TUTOR_UNAVAILABLE", "Репетитор временно недоступен.")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil {
		return training.TutorReply{}, tutorProviderError(err)
	}
	if len(body) > 64*1024 {
		return training.TutorReply{}, invalidTutorResponse()
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content   *string           `json:"content"`
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &completion) != nil || len(completion.Choices) != 1 || completion.Choices[0].Message.Content == nil || len(completion.Choices[0].Message.ToolCalls) != 0 || completion.Choices[0].FinishReason != "stop" {
		return training.TutorReply{}, invalidTutorResponse()
	}
	var result struct {
		AssistantText *string `json:"assistant_text"`
		Pedagogy      *string `json:"pedagogy"`
	}
	decoder := json.NewDecoder(strings.NewReader(*completion.Choices[0].Message.Content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.AssistantText == nil || result.Pedagogy == nil {
		return training.TutorReply{}, invalidTutorResponse()
	}
	text := strings.TrimSpace(*result.AssistantText)
	if text == "" || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || utf8.RuneCountInString(text) > 1500 {
		return training.TutorReply{}, invalidTutorResponse()
	}
	pedagogy := training.Pedagogy(*result.Pedagogy)
	switch pedagogy {
	case training.PedagogyQuestion, training.PedagogyHint, training.PedagogyConcept, training.PedagogyMisconception, training.PedagogyConfirmation, training.PedagogyClarification:
	default:
		return training.TutorReply{}, invalidTutorResponse()
	}
	return training.TutorReply{AssistantText: text, Pedagogy: pedagogy}, nil
}

var _ interface {
	Reply(context.Context, training.TutorContext) (training.TutorReply, error)
} = (*TutorClient)(nil)
