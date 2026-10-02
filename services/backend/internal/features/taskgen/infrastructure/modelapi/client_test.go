package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
)

func TestGenerateSendsSamplesAndParsesTasks(t *testing.T) {
	var request struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"tasks\":[{\"question\":\"Новое?\",\"criteria\":\"Верно.\",\"voice_instruction\":\"Ответьте.\",\"options\":[\"А\",\"Б\"]}]}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	client := New(server.Client(), server.URL+"/v1", "gpt-oss-120b", time.Second)
	source := application.Source{OutcomeID: "o-1", OutcomeName: "Дроби", ConstituentName: "Сост", CompetencyName: "ПК-1", Tasks: []application.Task{{Question: "Что обозначает дробь?", Criteria: "Критерий"}}}
	tasks, err := client.Generate(context.Background(), source, 1)
	if err != nil || len(tasks) != 1 || tasks[0].Question != "Новое?" || len(tasks[0].Options) != 2 {
		t.Fatalf("unexpected model result: %#v, %v", tasks, err)
	}
	if request.Model != "gpt-oss-120b" || len(request.Messages) != 2 || request.Messages[0].Role != "system" {
		t.Fatalf("model request shape: %#v", request)
	}
	user := request.Messages[1].Content
	if !strings.Contains(user, `"outcome":"Дроби"`) || !strings.Contains(user, `"count":1`) || !strings.Contains(user, `"question":"Что обозначает дробь?"`) {
		t.Fatalf("grounding data omitted: %s", user)
	}
}

func TestGenerateRejectsTruncatedModelResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null},"finish_reason":"length"}]}`))
	}))
	defer server.Close()
	_, err := New(server.Client(), server.URL+"/v1", "gpt-oss-120b", time.Second).Generate(context.Background(), application.Source{OutcomeID: "o-1"}, 1)
	if err == nil {
		t.Fatal("expected truncated completion to fail")
	}
}

func TestGenerateMapsProviderStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{503, "TASK_GENERATION_UNAVAILABLE"},
		{429, "TASK_GENERATION_UNAVAILABLE"},
		{504, "TASK_GENERATION_TIMEOUT"},
		{500, "TASK_GENERATION_FAILED"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		_, err := New(server.Client(), server.URL+"/v1", "gpt-oss-120b", time.Second).Generate(context.Background(), application.Source{OutcomeID: "o-1"}, 1)
		if err == nil || err.Error() != tc.code {
			t.Fatalf("status %d: got %v, want %s", tc.status, err, tc.code)
		}
		server.Close()
	}
}
