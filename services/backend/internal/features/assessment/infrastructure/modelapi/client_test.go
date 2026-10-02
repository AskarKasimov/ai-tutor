package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
)

func TestGradeSendsTaskAndParsesThreeLineResult(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"score\":2,\"feedback\":[\"Верно.\",\"Два класса.\",\"Закрепите тему.\"]}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	client := New(server.Client(), server.URL+"/v1", "gpt-oss-120b", time.Second)
	result, err := client.Grade(context.Background(), application.Task{Question: "Вопрос", Options: []string{"Классификация"}, VoiceInstruction: "Назовите тип", CorrectAnswer: "Классификация"}, "классификация")
	if err != nil || result.Score != 2 || len(result.Feedback) != 3 {
		t.Fatalf("unexpected model result: %#v, %v", result, err)
	}
	if request.Model != "gpt-oss-120b" || len(request.Messages) != 2 || request.Messages[0].Role != "system" || !strings.Contains(request.Messages[1].Content, `"student_answer":"классификация"`) {
		t.Fatalf("model request omitted task or answer: %#v", request)
	}
}

func TestGradeRejectsTruncatedModelResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null},"finish_reason":"length"}]}`))
	}))
	defer server.Close()
	_, err := New(server.Client(), server.URL+"/v1", "gpt-oss-120b", time.Second).Grade(context.Background(), application.Task{Question: "Вопрос"}, "ответ")
	if err == nil {
		t.Fatal("expected truncated completion to fail")
	}
}
