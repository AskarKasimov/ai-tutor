package llm

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	llmgatewayv1 "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
)

// fakeGateway answers Complete with scripted message contents in order; the
// last one repeats.
type fakeGateway struct {
	llmgatewayv1.UnimplementedGatewayServer
	mu        sync.Mutex
	requests  []*llmgatewayv1.CompleteRequest
	answers   []string
	err       error
	block     bool
	cancelled chan struct{}
	header    metadata.MD
}

func (f *fakeGateway) Complete(ctx context.Context, r *llmgatewayv1.CompleteRequest) (*llmgatewayv1.CompleteResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	n := len(f.requests)
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		close(f.cancelled)
		return nil, ctx.Err()
	}
	if f.header != nil {
		_ = grpc.SetHeader(ctx, f.header)
	}
	if f.err != nil {
		return nil, f.err
	}
	completion, _ := json.Marshal(map[string]any{
		"id": "c", "object": "chat.completion", "created": 1, "model": "gpt-oss-120b",
		"choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": f.answers[min(n, len(f.answers))-1]}, "finish_reason": "stop"}},
	})
	return &llmgatewayv1.CompleteResponse{RequestId: "req-1", CompletionJson: completion}, nil
}

func (f *fakeGateway) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func serve(t *testing.T, f *fakeGateway, timeout time.Duration) *Gateway {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	llmgatewayv1.RegisterGatewayServer(server, f)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	g, conn, err := Dial(listener.Addr().String(), timeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return g
}

var node = exam.Node{ID: "a-star", Title: "Алгоритм A*", Content: "f(n) = g(n) + h(n)."}

var turns = []exam.Turn{
	{Line: 1, Speaker: exam.SpeakerGeek, Text: "Вопрос?"},
	{Line: 2, Speaker: exam.SpeakerStudent, Text: "Ответ."},
}

func TestGatewayAsksWithComplexityAndJSONFormat(t *testing.T) {
	f := &fakeGateway{answers: []string{`{"text": "Почему A* оптимален?"}`}}
	g := serve(t, f, time.Second)
	text, err := g.Question(context.Background(), application.QuestionRequest{Kind: exam.KindRephrase, Node: node})
	if err != nil || text != "Почему A* оптимален?" {
		t.Fatalf("question = %q, %v", text, err)
	}
	r := f.requests[0]
	if r.GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_LOW || !r.GetUrgent() {
		t.Fatalf("complexity = %s urgent = %v", r.GetComplexity(), r.GetUrgent())
	}
	if r.GetParameters().GetResponseFormat() != llmgatewayv1.ResponseFormat_RESPONSE_FORMAT_JSON_OBJECT || r.GetParameters().GetTemperature() != 0.2 ||
		r.GetParameters().GetReasoningEffort() != llmgatewayv1.ReasoningEffort_REASONING_EFFORT_LOW {
		t.Fatalf("parameters = %v", r.GetParameters())
	}
	messages := r.GetMessages()
	if len(messages) != 2 || messages[0].GetRole() != llmgatewayv1.Role_ROLE_SYSTEM || messages[1].GetRole() != llmgatewayv1.Role_ROLE_USER ||
		!strings.Contains(messages[1].GetContent(), "f(n) = g(n) + h(n).") {
		t.Fatalf("messages = %v", messages)
	}
	if _, err := g.Question(context.Background(), application.QuestionRequest{Kind: exam.KindFollowup, Node: node}); err != nil ||
		f.requests[1].GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_LOW {
		t.Fatalf("followup complexity = %s, %v", f.requests[1].GetComplexity(), err)
	}
	if _, err := g.Question(context.Background(), application.QuestionRequest{Kind: exam.KindQuestion, Node: node}); err != nil ||
		f.requests[2].GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_LOW {
		t.Fatalf("main question asked in a session: complexity = %s, %v", f.requests[2].GetComplexity(), err)
	}
	if _, err := g.Question(context.Background(), application.QuestionRequest{Kind: exam.KindQuestion, Node: node, Ahead: true}); err != nil ||
		f.requests[3].GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_MEDIUM || f.requests[3].GetUrgent() {
		t.Fatalf("main question prepared ahead: complexity = %s, %v", f.requests[3].GetComplexity(), err)
	}
}

func TestGatewayEvaluatesAndRetriesAnInvalidAnswerOnce(t *testing.T) {
	f := &fakeGateway{answers: []string{"не JSON", "```json\n{\"verdict\":\"answered\",\"criteria\":{\"k1\":2,\"k2\":1,\"k3\":0,\"k4\":1},\"evidence_lines\":[2]}\n```"}}
	g := serve(t, f, time.Second)
	e, err := g.Evaluate(context.Background(), application.EvaluationRequest{Kind: exam.KindQuestion, Node: node, Dialogue: turns})
	if err != nil || e.Verdict != exam.VerdictAnswered || e.Criteria != (exam.Criteria{K1: 2, K2: 1, K4: 1}) || e.Evidence[0] != 2 {
		t.Fatalf("evaluation = %+v, %v", e, err)
	}
	if len(f.requests) != 2 || f.requests[0].GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_LOW || !f.requests[0].GetUrgent() {
		t.Fatalf("requests = %d", len(f.requests))
	}
	if !strings.Contains(f.requests[0].GetMessages()[1].GetContent(), "[2] Студент: Ответ.") {
		t.Fatal("prompt lacks the numbered dialogue")
	}
}

func TestGatewayReturnsTheReplyAndLeavesTheRecapOutOfTheDialogue(t *testing.T) {
	f := &fakeGateway{answers: []string{
		`{"verdict":"answered","criteria":{"k1":1,"k2":1,"k3":0,"k4":0},"evidence_lines":[2],"reaction":" Близко. ","explanation":"Близко. На самом деле f = g + h."}`,
		`{"verdict":"answered","criteria":{"k1":1,"k2":1,"k3":0,"k4":0},"evidence_lines":[2]}`,
	}}
	g := serve(t, f, time.Second)
	recap := "Разбор прошлой темы.\n\n"
	dialogue := []exam.Turn{
		{Line: 1, Speaker: exam.SpeakerGeek, Text: recap + "Вопрос?", Recap: recap},
		{Line: 2, Speaker: exam.SpeakerStudent, Text: "Ответ."},
	}
	request := application.EvaluationRequest{Kind: exam.KindQuestion, Node: node, Prerequisites: []string{"Эвристика"}, Dialogue: dialogue}
	a, err := g.Evaluate(context.Background(), request)
	if err != nil || a.Reaction != "Близко." || a.Explanation != "Близко. На самом деле f = g + h." || a.Verdict != exam.VerdictAnswered {
		t.Fatalf("assessment = %+v, %v", a, err)
	}
	prompt := f.requests[0].GetMessages()[1].GetContent()
	if strings.Contains(prompt, "Разбор прошлой темы") || !strings.Contains(prompt, "[1] Geek: Вопрос?") || !strings.Contains(prompt, "Эвристика") {
		t.Fatalf("prompt = %s", prompt)
	}
	// A reply is optional: without it Geek just asks on.
	if a, err := g.Evaluate(context.Background(), request); err != nil || a.Reaction != "" || a.Explanation != "" {
		t.Fatalf("assessment without a reply = %+v, %v", a, err)
	}
}

func TestEvaluationGradesByOutcomesNotBySlides(t *testing.T) {
	// Decision of 18.10.2026: the outcomes say what to check, the topic text is
	// the reference by meaning, and slide details are never required.
	n := node
	n.Outcomes = []string{"объяснять, из чего складывается оценка f(n)"}
	prompt := evaluationPrompt(application.EvaluationRequest{Kind: exam.KindQuestion, Node: n, Dialogue: turns})
	for _, want := range []string{
		"- объяснять, из чего складывается оценка f(n)",
		"достиг ли студент образовательных результатов темы",
		"по сути, не дословно",
		"номер слайда или лекции",
		"выбирай более высокий",
		"одно предложение до 15 слов",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("evaluation prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "строго по тексту темы") || strings.Contains(prompt, "35 слов") {
		t.Error("evaluation prompt still grades strictly by the topic text or allows 35 words")
	}
	if p := explanationPrompt(application.EvaluationRequest{Node: n, Dialogue: turns}); !strings.Contains(p, "одно предложение до 15 слов") {
		t.Errorf("explanation prompt = %s", p)
	}
}

func TestGatewayExplainsOnTheLowPool(t *testing.T) {
	f := &fakeGateway{answers: []string{`{"text": "A* складывает g и h."}`}}
	g := serve(t, f, time.Second)
	text, err := g.Explain(context.Background(), application.EvaluationRequest{Node: node, Prerequisites: []string{"Эвристика"}, Dialogue: turns})
	if err != nil || text != "A* складывает g и h." {
		t.Fatalf("explanation = %q, %v", text, err)
	}
	r := f.requests[0]
	if r.GetComplexity() != llmgatewayv1.Complexity_COMPLEXITY_LOW || !r.GetUrgent() || !strings.Contains(r.GetMessages()[1].GetContent(), "Эвристика") {
		t.Fatalf("complexity = %s prompt = %s", r.GetComplexity(), r.GetMessages()[1].GetContent())
	}
}

func TestGatewayDoesNotReuseFieldsOfARejectedAnswer(t *testing.T) {
	f := &fakeGateway{answers: []string{
		`{"verdict":"answered","criteria":{"k1":2,"k2":2,"k3":2,"k4":3},"evidence_lines":[2]}`,
		`{"verdict":"answered","criteria":{"k4":2},"evidence_lines":[2]}`,
	}}
	g := serve(t, f, time.Second)
	if e, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: node, Dialogue: turns}); err == nil {
		t.Fatalf("the second answer lacks K1-K3 but was accepted as %+v", e)
	}
	if len(f.requests) != 2 {
		t.Fatalf("requests = %d", len(f.requests))
	}
}

func TestGatewayFailureIsNotRetried(t *testing.T) {
	f := &fakeGateway{err: status.Error(codes.Unavailable, "no slots")}
	g := serve(t, f, time.Second)
	_, err := g.Question(context.Background(), application.QuestionRequest{Node: node})
	if status.Code(err) != codes.Unavailable || len(f.requests) != 1 {
		t.Fatalf("err = %v after %d calls", err, len(f.requests))
	}
}

func TestFailedCallKeepsTheGatewayRequestID(t *testing.T) {
	request := &llmgatewayv1.CompleteRequest{Complexity: llmgatewayv1.Complexity_COMPLEXITY_LOW}
	f := &fakeGateway{err: status.Error(codes.Unavailable, "no slots"), header: metadata.Pairs("x-request-id", "gateway-error-42")}
	if _, id, err := serve(t, f, time.Second).call(context.Background(), request); err == nil || id != "gateway-error-42" {
		t.Fatalf("header: id = %q, err = %v", id, err)
	}
	detailed, _ := status.New(codes.ResourceExhausted, "queue full").WithDetails(&errdetails.ErrorInfo{Reason: "QUEUE_FULL", Metadata: map[string]string{"request_id": "gateway-error-43"}})
	f = &fakeGateway{err: detailed.Err()}
	if _, id, err := serve(t, f, time.Second).call(context.Background(), request); err == nil || id != "gateway-error-43" {
		t.Fatalf("error info: id = %q, err = %v", id, err)
	}
}

func TestCancellingTheRequestStopsItInTheGateway(t *testing.T) {
	f := &fakeGateway{block: true, cancelled: make(chan struct{})}
	g := serve(t, f, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := g.Question(ctx, application.QuestionRequest{Node: node})
		done <- err
	}()
	for f.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-f.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the gateway did not see the cancellation")
	}
	if err := <-done; err == nil {
		t.Fatal("a cancelled request succeeded")
	}
}
