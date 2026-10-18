package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	geekllm "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/llm"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEvaluationUsesProductionPromptParserAndTransitions(t *testing.T) {
	calls := 0
	tr := &transport{endpoint: "http://eval.invalid/v1", model: "test-model", reasoning: "production"}
	tr.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body struct {
			Model     string `json:"model"`
			Reasoning string `json:"reasoning_effort"`
			Messages  []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Reasoning != "low" {
			t.Fatalf("production grading reasoning = %q", body.Reasoning)
		}
		if body.Model != "test-model" || len(body.Messages) != 2 || !strings.Contains(body.Messages[1].Content, "TEXT_FROM_GRAPH") || strings.Contains(body.Messages[1].Content, "OUTCOME_MUST_NOT_BE_SENT") {
			t.Fatalf("wrong request: %+v", body)
		}
		content := `{"verdict":"answered","answer_complete":true,"criteria":{"k1":2,"k2":2,"k3":null,"k4":null},"evidence_lines":[2],"reaction":"Хорошо.","explanation":"Суть верна."}`
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	g := geekllm.NewGateway(tr, time.Second)
	graph := exam.Graph{Nodes: []exam.Node{{ID: "n", Title: "Topic", Content: "TEXT_FROM_GRAPH", Outcomes: []string{"OUTCOME_MUST_NOT_BE_SENT"}}}}
	r := evaluateCase(context.Background(), g, tr, graph, testCase{ID: "good", NodeID: "n", Question: "Question?", Answers: []string{"Answer."}})
	if r.Error != "" || calls != 1 || r.State == nil || *r.State.Grade != exam.GradeA || r.Next.Kind != exam.KindClosing {
		t.Fatalf("result=%+v calls=%d", r, calls)
	}
	if !r.State.Unassessed.K3 || !r.State.Unassessed.K4 || len(tr.calls) != 1 || len(tr.calls[0].Response) == 0 {
		t.Fatal("scope or raw model response lost")
	}
}

func TestPreviewExportsPromptWithoutModelOrInventedGrade(t *testing.T) {
	tr := &transport{preview: true}
	g := geekllm.NewGateway(tr, time.Second)
	graph := exam.Graph{Nodes: []exam.Node{{ID: "n", Title: "Topic", Content: "TEXT_FROM_GRAPH"}}}
	r := evaluateCase(context.Background(), g, tr, graph, testCase{ID: "preview", NodeID: "n", Question: "Question?", Answers: []string{"Answer."}})
	if r.Error != "" || r.State != nil || len(r.Assessments) != 0 || len(tr.calls) != 1 {
		t.Fatalf("preview fabricated a result: %+v", r)
	}
	if !strings.Contains(string(tr.calls[0].Request), "TEXT_FROM_GRAPH") {
		t.Fatal("preview lacks production prompt")
	}
}
