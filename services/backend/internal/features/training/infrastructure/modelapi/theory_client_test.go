package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

func TestTheoryClientSendsEveryNoteAndReadsSummary(t *testing.T) {
	var body struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		content, _ := json.Marshal(map[string]string{"theory": "  Выжимка  "})
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	client := NewTheory(server.Client(), server.URL+"/v1/", "local-model", time.Second)
	text, err := client.Summarize(context.Background(), training.TheoryRequest{SubjectName: "ML", CompetencyName: "Регрессия", MainOutcome: "Применяет", Notes: []training.TheoryNote{
		{OutcomeName: "В тесте", IncludeInTest: true, Content: "ОС один"},
		{OutcomeName: "Вне теста", Content: "ОС два"},
	}})
	if err != nil || text != "Выжимка" {
		t.Fatalf("summary %q %v", text, err)
	}
	if body.Model != "local-model" || len(body.Messages) != 2 || body.Messages[0].Role != "system" {
		t.Fatalf("request %+v", body)
	}
	data := body.Messages[1].Content
	for _, want := range []string{TheoryPromptVersion, "ОС один", "ОС два", `"include_in_test":false`, "Регрессия"} {
		if !strings.Contains(data, want) {
			t.Fatalf("theory request misses %q: %s", want, data)
		}
	}
}

func TestTheoryClientRejectsMalformedSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"theory":""}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	if _, err := NewTheory(server.Client(), server.URL, "m", time.Second).Summarize(context.Background(), training.TheoryRequest{}); err == nil {
		t.Fatal("empty theory accepted")
	}
}
