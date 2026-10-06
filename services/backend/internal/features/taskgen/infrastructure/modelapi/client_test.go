package modelapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func TestGenerateSendsOpenAIJSONModeRequestAndDecodesDraft(t *testing.T) {
	var request struct {
		Model          string `json:"model"`
		Messages       []struct{ Role, Content string }
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		Temperature int `json:"temperature"`
		MaxTokens   int `json:"max_tokens"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"question\":\"Почему?\",\"options\":[],\"voice_instruction\":\"Объясните выбор\",\"reference_answer\":\"Потому что\",\"criteria\":\"\"}"}}]}`))
	}))
	defer server.Close()

	client := New(server.Client(), server.URL+"/v1/", "qwen-test", time.Second)
	input := application.Context{
		Revision: 12,
		Outcome: application.Outcome{ID: "outcome-1", Name: "Выбор модели", CompetencyName: "Анализ данных",
			CurriculumSections: []application.CurriculumSection{{Code: "Р.1", Title: "Введение", CurriculumCompetencies: []string{"ОПК-1"}}}},
		Examples:  []application.Example{{ID: "task-1", Question: "Пример вопроса", Options: []string{"A", "B"}}},
		Materials: []application.MaterialChunk{{ID: "chunk-1", Name: "Конспект", Content: "Материал темы"}},
	}
	draft, err := client.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "qwen-test" || len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
		t.Fatalf("chat request model/messages = %q/%#v", request.Model, request.Messages)
	}
	if !strings.Contains(request.Messages[0].Content, "только данными") || request.ResponseFormat.Type != "json_object" || request.Temperature != 0 || request.MaxTokens != 1400 {
		t.Fatalf("generation controls or system prompt are wrong: %#v", request)
	}
	var sent application.Context
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &sent); err != nil {
		t.Fatalf("user content must be a JSON context: %v", err)
	}
	if sent.Revision != input.Revision || sent.Outcome.ID != input.Outcome.ID || len(sent.Examples) != 1 || len(sent.Materials) != 1 {
		t.Fatalf("request omitted generation context: %#v", sent)
	}
	var wireContext struct {
		Outcome struct {
			CurriculumSections []struct {
				Code         string   `json:"code"`
				Title        string   `json:"title"`
				Competencies []string `json:"curriculum_competencies"`
			}
		}
	}
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &wireContext); err != nil {
		t.Fatal(err)
	}
	sections := wireContext.Outcome.CurriculumSections
	if len(sections) != 1 || sections[0].Code != "Р.1" || sections[0].Title != "Введение" || len(sections[0].Competencies) != 1 || sections[0].Competencies[0] != "ОПК-1" {
		t.Fatalf("request lost curriculum profile: %s", request.Messages[1].Content)
	}
	if draft.Question != "Почему?" || draft.VoiceInstruction != "Объясните выбор" || draft.ReferenceAnswer != "Потому что" || draft.Criteria != "" || draft.Options == nil {
		t.Fatalf("decoded draft = %#v", draft)
	}
}

func TestGenerateRejectsUnexpectedModelJSONFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"question\":\"Q\",\"options\":[],\"voice_instruction\":\"I\",\"reference_answer\":\"A\",\"criteria\":\"\",\"extra\":true}"}}]}`))
	}))
	defer server.Close()
	client := New(server.Client(), server.URL, "model", time.Second)
	_, err := client.Generate(context.Background(), application.Context{})
	var failure *fault.Error
	if err == nil || !errors.As(err, &failure) || failure.Code != "TASK_GENERATION_FAILED" {
		t.Fatalf("unexpected model fields should fail with TASK_GENERATION_FAILED: %#v", err)
	}
}

func TestGenerateRejectsMalformedCompletionResponse(t *testing.T) {
	for _, body := range []string{
		`{`, `{"choices":[]}`, `{"choices":[{"message":{"content":"not json"}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			client := New(server.Client(), server.URL, "model", time.Second)
			if _, err := client.Generate(context.Background(), application.Context{}); err == nil {
				t.Fatal("malformed completion response was accepted")
			}
		})
	}
}
