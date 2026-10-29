package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	llmgatewayv1 "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// Gateway calls the LLM gateway over gRPC (llmgateway.v1.Gateway/Complete):
// no model is chosen by the client, only the complexity. Cancelling the
// context, for example when the student leaves, stops the queued or running
// request in the gateway.
type Gateway struct {
	client  llmgatewayv1.GatewayClient
	timeout time.Duration
}

func NewGateway(client llmgatewayv1.GatewayClient, timeout time.Duration) *Gateway {
	return &Gateway{client: client, timeout: timeout}
}

// Dial connects lazily to the gateway at host:port inside the trusted network.
func Dial(address string, timeout time.Duration) (*Gateway, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(40<<20)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connect LLM gateway %s: %w", address, err)
	}
	return NewGateway(llmgatewayv1.NewGatewayClient(conn), timeout), conn, nil
}

func (g *Gateway) Question(ctx context.Context, r application.QuestionRequest) (string, error) {
	// Everything asked during a session uses the fast low pool (decision of
	// 18.10.2026); only a main question or rephrase prepared ahead may take
	// medium, and it is not urgent.
	level := llmgatewayv1.Complexity_COMPLEXITY_LOW
	if r.Ahead {
		level = llmgatewayv1.Complexity_COMPLEXITY_MEDIUM
	}
	var text string
	operation := "question:" + string(r.Kind)
	if r.Hint {
		operation = "hint_context"
	}
	err := g.complete(ctx, operation, level, !r.Ahead, questionPrompt(r), func(content []byte) error {
		var out struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(content, &out); err != nil {
			return err
		}
		if text = strings.TrimSpace(out.Text); text == "" {
			return errors.New("empty text")
		}
		if r.Hint && strings.Contains(text, "?") {
			return errors.New("context must not generate a leading question")
		}
		return nil
	})
	if err == nil && r.Hint {
		question := r.Node.MainQuestion()
		if question == "" {
			question, _ = lastSupportInput(application.EvaluationRequest{Node: r.Node, Dialogue: r.Dialogue})
		}
		if question != "" {
			text += "\n\n" + question
		}
	}
	return text, err
}

func (g *Gateway) Evaluate(ctx context.Context, r application.EvaluationRequest) (application.Assessment, error) {
	spec, specErr := exam.ParseRubric(r.Node.AssessmentGuidance)
	if specErr != nil {
		return application.Assessment{}, specErr
	}
	if len(spec) > 0 {
		return g.evaluateRubric(ctx, r, spec)
	}
	var result application.Assessment
	// The student waits for the evaluation, so it uses the fast low pool too.
	err := g.complete(ctx, "evaluate:"+string(r.Kind), llmgatewayv1.Complexity_COMPLEXITY_LOW, true, evaluationPrompt(r), func(content []byte) error {
		var out struct {
			AnswerComplete *bool        `json:"answer_complete"`
			Verdict        exam.Verdict `json:"verdict"`
			Criteria       *struct {
				K1 *int            `json:"k1"`
				K2 *int            `json:"k2"`
				K3 json.RawMessage `json:"k3"`
				K4 json.RawMessage `json:"k4"`
			} `json:"criteria"`
			Evidence          []json.Number          `json:"evidence_lines"`
			AddressPreference exam.AddressPreference `json:"address_preference"`
		}
		if err := json.Unmarshal(content, &out); err != nil {
			return err
		}
		// Models sometimes quote line numbers. Accept the same integer value,
		// while rejecting fractions, non-numeric strings and overflow.
		e := exam.Evaluation{Verdict: out.Verdict}
		for _, number := range out.Evidence {
			line, err := strconv.Atoi(number.String())
			if err != nil {
				return fmt.Errorf("invalid evidence line: %w", err)
			}
			e.Evidence = append(e.Evidence, line)
		}
		switch out.Verdict {
		case exam.VerdictAnswered:
			if out.AnswerComplete == nil {
				return errors.New("missing answer_complete")
			}
			e.AnswerComplete = out.AnswerComplete
			k := out.Criteria
			if k == nil || k.K1 == nil || k.K2 == nil || len(k.K3) == 0 || len(k.K4) == 0 {
				return errors.New("missing criteria")
			}
			e.Criteria = exam.Criteria{K1: *k.K1, K2: *k.K2}
			for _, field := range []struct {
				raw        json.RawMessage
				value      *int
				unassessed *bool
			}{
				{k.K3, &e.Criteria.K3, &e.Unassessed.K3},
				{k.K4, &e.Criteria.K4, &e.Unassessed.K4},
			} {
				if string(field.raw) == "null" {
					*field.unassessed = true
				} else if err := json.Unmarshal(field.raw, field.value); err != nil {
					return err
				}
			}
			if !e.Criteria.Valid() {
				return fmt.Errorf("criteria out of range: %+v", e.Criteria)
			}
		case exam.VerdictDontKnow, exam.VerdictInsufficient, exam.VerdictHintRequested, exam.VerdictAddressCorrection, exam.VerdictClarifyQuestion, exam.VerdictPause:
		default:
			return fmt.Errorf("unknown verdict %q", out.Verdict)
		}
		if !out.AddressPreference.Valid() || (out.Verdict == exam.VerdictAddressCorrection && out.AddressPreference == "") {
			return errors.New("invalid address preference")
		}
		// Never show grading-model prose to a student, even if the model emits it.
		result = application.Assessment{Evaluation: e, AddressPreference: out.AddressPreference}
		return nil
	})
	if err != nil {
		return application.Assessment{}, err
	}
	if result.Verdict == exam.VerdictClarifyQuestion {
		if err := g.clarifySupport(ctx, r, &result); err != nil {
			return application.Assessment{}, err
		}
	}
	if err := g.checkIndependence(ctx, r, &result); err != nil {
		return application.Assessment{}, err
	}
	return result, nil
}

// Feedback is generated only after the engine has chosen the next step.
func (g *Gateway) Feedback(ctx context.Context, r application.FeedbackRequest) (string, error) {
	if !r.Closed && r.Verdict == exam.VerdictHintRequested {
		return transition(r, hintTransitions), nil
	}
	// A rephrase needs a transition, not another model-generated question or
	// hints. Keep this branch deterministic, including the speaker's agency.
	if !r.Closed && r.Next == exam.KindRephrase {
		if r.Verdict == exam.VerdictInsufficient {
			return transition(r, offTopicTransitions), nil
		}
		return transition(r, rephraseTransitions), nil
	}
	if !r.Closed {
		if r.Result.Rubric != nil {
			if r.Result.Rubric.Complete() {
				return transition(r, soundFollowupTransitions), nil
			}
			return transition(r, partialFollowupTransitions), nil
		}
		// Only the actual followup asks for the missing knowledge. Generating
		// a second reaction with the same model repeatedly leaked that answer.
		if k := r.Result.Criteria; k != nil && k.K1 == 2 && k.K2 == 2 {
			return transition(r, soundFollowupTransitions), nil
		}
		return transition(r, partialFollowupTransitions), nil
	}
	var text string
	err := g.complete(ctx, "feedback", llmgatewayv1.Complexity_COMPLEXITY_LOW, true, feedbackPrompt(r), func(content []byte) error {
		var out struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(content, &out); err != nil {
			return err
		}
		if text = strings.TrimSpace(out.Text); text == "" {
			return errors.New("empty text")
		}
		limit := 80
		if len(strings.Fields(text)) > limit {
			return fmt.Errorf("feedback exceeds %d words", limit)
		}
		return nil
	})
	return text, err
}

// complete asks for a JSON object and hands each answer to parse, which
// decodes it into a fresh value and checks it; an invalid answer is retried once.
// A call the student waits for is urgent: the gateway runs it before queued
// background work (graph building, translation) on any free compatible instance.
func (g *Gateway) complete(ctx context.Context, operation string, level llmgatewayv1.Complexity, urgent bool, prompt string, parse func([]byte) error) error {
	logger := httpx.LoggerFromContext(ctx).With(zap.String("llm_operation", operation), zap.String("complexity", level.String()), zap.Bool("urgent", urgent))
	system := systemPrompt
	if operation == "route_rubric" {
		system = rubricRouterSystemPrompt
	}
	if operation == "review_rubric" {
		system = rubricReviewSystemPrompt
	}
	if strings.HasPrefix(operation, "evaluate_rubric:") {
		system = rubricSystemPrompt
	}
	if operation == "feedback" {
		system = feedbackSystemPrompt
	}
	if operation == "support" {
		system = supportSystemPrompt
	}
	if operation == "independence" {
		system = independenceSystemPrompt
	}
	if operation == "hint_context" {
		system = contextSystemPrompt
	}
	request := &llmgatewayv1.CompleteRequest{
		Messages: []*llmgatewayv1.Message{
			{Role: llmgatewayv1.Role_ROLE_SYSTEM, Content: proto.String(system)},
			{Role: llmgatewayv1.Role_ROLE_USER, Content: proto.String(prompt)},
		},
		Complexity: level,
		Urgent:     urgent,
		Parameters: &llmgatewayv1.GenerationParameters{
			Temperature:     proto.Float64(0.2),
			ResponseFormat:  llmgatewayv1.ResponseFormat_RESPONSE_FORMAT_JSON_OBJECT.Enum(),
			ReasoningEffort: llmgatewayv1.ReasoningEffort_REASONING_EFFORT_LOW.Enum(),
		},
	}
	var last error
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		content, requestID, err := g.call(ctx, request)
		logger.Info("llm call", zap.Int("attempt", attempt), zap.String("gateway_request_id", requestID),
			zap.Duration("duration", time.Since(started)), zap.String("prompt", prompt), zap.String("response", content), zap.Error(err))
		if err != nil {
			return err
		}
		if err = parse([]byte(stripFence(content))); err == nil {
			return nil
		}
		last = fmt.Errorf("invalid model answer: %w", err)
		if attempt == 1 && (strings.HasPrefix(operation, "evaluate_rubric:") || operation == "review_rubric") {
			request.Messages = append(request.Messages,
				&llmgatewayv1.Message{Role: llmgatewayv1.Role_ROLE_ASSISTANT, Content: proto.String(content)},
				&llmgatewayv1.Message{Role: llmgatewayv1.Role_ROLE_USER, Content: proto.String("Исправь предыдущий JSON: " + err.Error() + ". Верни весь результат по исходной схеме, включая answer_complete и все пункты marks для answered. Не меняй шкалу, используй реальные строки студента. Для служебного verdict marks должны быть пустыми.")},
			)
		}
		if operation == "feedback" && attempt == 1 {
			request.Messages = append(request.Messages,
				&llmgatewayv1.Message{Role: llmgatewayv1.Role_ROLE_ASSISTANT, Content: proto.String(content)},
				&llmgatewayv1.Message{Role: llmgatewayv1.Role_ROLE_USER, Content: proto.String("Исправь формат предыдущей реплики: " + err.Error() + ". Верни только JSON с text; при открытой теме — реакция без вопроса или задания, затем приложение добавит вопрос.")},
			)
		}
	}
	return last
}

func (g *Gateway) call(ctx context.Context, request *llmgatewayv1.CompleteRequest) (content, requestID string, err error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	var header metadata.MD
	response, err := g.client.Complete(ctx, request, grpc.Header(&header))
	if err != nil {
		return "", failedRequestID(header, err), fmt.Errorf("call LLM gateway: %w", err)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(response.GetCompletionJson(), &completion); err != nil || len(completion.Choices) == 0 {
		return string(response.GetCompletionJson()), response.GetRequestId(), errors.New("LLM gateway returned no choices")
	}
	choice := completion.Choices[0]
	if choice.FinishReason == "length" {
		return choice.Message.Content, response.GetRequestId(), errors.New("LLM answer was cut by the token limit")
	}
	return choice.Message.Content, response.GetRequestId(), nil
}

// failedRequestID is the gateway request of a failed call: from the
// x-request-id header, else from the request_id of the error's ErrorInfo.
// A transport failure before the gateway answered has none.
func failedRequestID(header metadata.MD, err error) string {
	if ids := header.Get("x-request-id"); len(ids) > 0 {
		return ids[0]
	}
	if s, ok := status.FromError(err); ok {
		for _, detail := range s.Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok && info.GetMetadata()["request_id"] != "" {
				return info.GetMetadata()["request_id"]
			}
		}
	}
	return ""
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
