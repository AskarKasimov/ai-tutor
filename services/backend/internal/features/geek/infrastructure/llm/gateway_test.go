package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
)

func gatewayServer(t *testing.T, answers ...string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	requests := &[]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		*requests = append(*requests, body)
		answer := answers[min(len(*requests), len(answers))-1]
		if answer == "500" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": answer}, "finish_reason": "stop"}}})
	}))
	t.Cleanup(server.Close)
	return server, requests
}

var node = exam.Node{ID: "a-star", Title: "Алгоритм A*", Content: "f(n) = g(n) + h(n)."}

func TestGatewayAsksWithComplexityAndWithoutModel(t *testing.T) {
	server, requests := gatewayServer(t, `{"text": "Почему A* оптимален?"}`)
	g := NewGateway(server.Client(), server.URL+"/", time.Second)
	text, err := g.Question(context.Background(), application.QuestionRequest{Kind: exam.KindRephrase, Node: node})
	if err != nil || text != "Почему A* оптимален?" {
		t.Fatalf("question = %q, %v", text, err)
	}
	body := (*requests)[0]
	if _, ok := body["model"]; ok {
		t.Fatal("model must not be sent")
	}
	if body["complexity"] != "low" || body["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatalf("request = %v", body)
	}
	if !strings.Contains(body["messages"].([]any)[1].(map[string]any)["content"].(string), "f(n) = g(n) + h(n).") {
		t.Fatal("topic text is missing from the prompt")
	}
}

func TestGatewayEvaluatesAndRetriesAnInvalidAnswerOnce(t *testing.T) {
	server, requests := gatewayServer(t, "не JSON", "```json\n{\"verdict\":\"answered\",\"criteria\":{\"k1\":2,\"k2\":1,\"k3\":0,\"k4\":1},\"evidence_lines\":[2]}\n```")
	g := NewGateway(server.Client(), server.URL, time.Second)
	turns := []exam.Turn{{Line: 1, Speaker: exam.SpeakerGeek, Text: "Вопрос?"}, {Line: 2, Speaker: exam.SpeakerStudent, Text: "Ответ."}}
	e, err := g.Evaluate(context.Background(), application.EvaluationRequest{Kind: exam.KindQuestion, Node: node, Dialogue: turns})
	if err != nil || e.Verdict != exam.VerdictAnswered || e.Criteria != (exam.Criteria{K1: 2, K2: 1, K4: 1}) || e.Evidence[0] != 2 {
		t.Fatalf("evaluation = %+v, %v", e, err)
	}
	if len(*requests) != 2 || (*requests)[0]["complexity"] != "high" {
		t.Fatalf("requests = %d", len(*requests))
	}
	prompt := (*requests)[0]["messages"].([]any)[1].(map[string]any)["content"].(string)
	if !strings.Contains(prompt, "[2] Студент: Ответ.") {
		t.Fatalf("prompt lacks numbered dialogue: %s", prompt)
	}
}

func TestGatewayRejectsInvalidCriteriaAndFailures(t *testing.T) {
	server, _ := gatewayServer(t, `{"verdict":"answered","criteria":{"k1":3,"k2":1,"k3":0,"k4":1}}`)
	g := NewGateway(server.Client(), server.URL, time.Second)
	turns := []exam.Turn{{Line: 1, Speaker: exam.SpeakerStudent, Text: "Ответ."}}
	if _, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: node, Dialogue: turns}); err == nil {
		t.Fatal("criteria out of range accepted")
	}
	failing, requests := gatewayServer(t, "500")
	if _, err := NewGateway(failing.Client(), failing.URL, time.Second).Question(context.Background(), application.QuestionRequest{Node: node}); err == nil || len(*requests) != 1 {
		t.Fatalf("gateway failure: %v after %d calls (no retry of transport errors)", err, len(*requests))
	}
}

func TestMockRecognisesDontKnowAndGradesByLength(t *testing.T) {
	ctx := context.Background()
	for text, want := range map[string]exam.Verdict{"Честно, не знаю": exam.VerdictDontKnow, "Ну это поиск": exam.VerdictAnswered} {
		e, _ := Mock{}.Evaluate(ctx, application.EvaluationRequest{Node: node, Dialogue: []exam.Turn{{Line: 1, Speaker: exam.SpeakerStudent, Text: text}}})
		if e.Verdict != want {
			t.Fatalf("%q -> %s", text, e.Verdict)
		}
	}
	long := strings.Repeat("эвристика оценивает путь до цели ", 4)
	student := exam.SpeakerStudent
	e, _ := Mock{}.Evaluate(ctx, application.EvaluationRequest{Node: node, Dialogue: []exam.Turn{{Line: 3, Speaker: student, Text: long}}})
	if exam.TopicGrade(e.Criteria, false) != exam.GradeA || e.Evidence[0] != 3 {
		t.Fatalf("long answer = %+v", e)
	}
	// A strong main answer keeps K1 and K2 after a weak followup.
	e, _ = Mock{}.Evaluate(ctx, application.EvaluationRequest{Kind: exam.KindFollowup, Node: node, Dialogue: []exam.Turn{
		{Line: 1, Speaker: exam.SpeakerGeek, Text: "Вопрос?"}, {Line: 2, Speaker: student, Text: long},
		{Line: 3, Speaker: exam.SpeakerGeek, Text: "А если?"}, {Line: 4, Speaker: student, Text: "Ну это поиск"},
	}})
	if e.Criteria != (exam.Criteria{K1: 2, K2: 2}) || exam.TopicGrade(e.Criteria, false) != exam.GradeC || len(e.Evidence) != 2 {
		t.Fatalf("whole topic = %+v", e)
	}
}

func TestGatewayDoesNotReuseFieldsOfARejectedAnswer(t *testing.T) {
	server, requests := gatewayServer(t,
		`{"verdict":"answered","criteria":{"k1":2,"k2":2,"k3":2,"k4":3},"evidence_lines":[2]}`,
		`{"verdict":"answered","criteria":{"k4":2},"evidence_lines":[2]}`)
	g := NewGateway(server.Client(), server.URL, time.Second)
	turns := []exam.Turn{{Line: 1, Speaker: exam.SpeakerGeek, Text: "Вопрос?"}, {Line: 2, Speaker: exam.SpeakerStudent, Text: "Ответ."}}
	if e, err := g.Evaluate(context.Background(), application.EvaluationRequest{Node: node, Dialogue: turns}); err == nil {
		t.Fatalf("the second answer lacks K1-K3 but was accepted as %+v", e)
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %d", len(*requests))
	}
}
