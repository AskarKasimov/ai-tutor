package modelapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func validTutorContext() training.TutorContext {
	return training.TutorContext{
		Exercise: training.Exercise{ID: "exercise-1", Task: training.Task{Question: "Определите нужный результат", Options: []string{"число", "класс"}, VoiceInstruction: "Объясните выбор", ReferenceAnswer: "закрытая формулировка", Criteria: "обосновать признак", OutcomeName: "учебная цель", EducationalContent: "предметный материал"}},
		Summary:  "2: ученик выделил данные", Recent: []training.ChatMessage{{Role: training.ChatRoleAssistant, Text: "Что известно?"}, {Role: training.ChatRoleUser, Text: "есть две группы"}},
		StudentText: "модель выдаёт число", Kind: training.ChatKindMessage,
	}
}

func TestTutorSendsFixedSystemPromptAndStudentHistoryAsDataMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model          string  `json:"model"`
			Temperature    float64 `json:"temperature"`
			MaxTokens      int     `json:"max_tokens"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "tutor-model" || body.Temperature != 0.3 || body.MaxTokens != 2048 || body.ResponseFormat.Type != "json_object" {
			t.Errorf("unexpected request settings: %+v", body)
		}
		roles := make([]string, len(body.Messages))
		for i, message := range body.Messages {
			roles[i] = message.Role
		}
		if fmt.Sprint(roles) != "[system user user assistant user user]" {
			t.Errorf("untrusted history roles were promoted: %v", roles)
		}
		if !strings.Contains(body.Messages[0].Content, "терпеливый русскоязычный учебный репетитор") || strings.Contains(body.Messages[0].Content, "закрытая формулировка") {
			t.Errorf("system message is not the fixed policy: %s", body.Messages[0].Content)
		}
		if !strings.Contains(body.Messages[1].Content, "закрытая формулировка") || !strings.Contains(body.Messages[1].Content, "training-tutor-v3") {
			t.Errorf("trusted snapshot envelope missing: %s", body.Messages[1].Content)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"assistant_text\":\"Какой результат должна выдавать модель?\",\"pedagogy\":\"question\",\"answer_ready\":false}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	client := NewTutor(server.Client(), server.URL, "tutor-model", time.Second)
	got, err := client.Reply(context.Background(), validTutorContext())
	if err != nil || got.Pedagogy != training.PedagogyQuestion || got.AssistantText != "Какой результат должна выдавать модель?" {
		t.Fatalf("unexpected tutor reply: %+v %v", got, err)
	}
	if TutorPromptVersion != "training-tutor-v3" {
		t.Fatalf("unexpected prompt version %q", TutorPromptVersion)
	}
}

func TestTutorRejectsMalformedOrUnsafeStructuredResponses(t *testing.T) {
	tests := map[string]struct{ content, finish string }{
		"missing readiness": {`{"assistant_text":"Подумайте","pedagogy":"hint"}`, "stop"},
		"string readiness":  {`{"assistant_text":"Подумайте","pedagogy":"hint","answer_ready":"true"}`, "stop"},
		"null readiness":    {`{"assistant_text":"Подумайте","pedagogy":"hint","answer_ready":null}`, "stop"},
		"malformed":         {"{", "stop"},
		"unknown field":     {`{"assistant_text":"Подумайте","pedagogy":"hint","score":2}`, "stop"},
		"empty":             {`{"assistant_text":"  ","pedagogy":"question","answer_ready":false}`, "stop"},
		"unknown pedagogy":  {`{"assistant_text":"Подумайте","pedagogy":"solution","answer_ready":false}`, "stop"},
		"too long":          {fmt.Sprintf(`{"assistant_text":%q,"pedagogy":"hint","answer_ready":false}`, strings.Repeat("я", 1501)), "stop"},
		"truncated finish":  {`{"assistant_text":"Подумайте","pedagogy":"hint","answer_ready":false}`, "length"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":%q}]}`, tc.content, tc.finish)
			}))
			defer server.Close()
			_, err := NewTutor(server.Client(), server.URL, "model", time.Second).Reply(context.Background(), validTutorContext())
			var f *fault.Error
			if !errors.As(err, &f) || f.Code != "INVALID_TUTOR_RESPONSE" {
				t.Fatalf("expected invalid tutor response, got %v", err)
			}
		})
	}
}

func TestTutorMapsProviderFailuresWithoutExposingBody(t *testing.T) {
	for status, code := range map[int]string{http.StatusTooManyRequests: "TUTOR_UNAVAILABLE", http.StatusServiceUnavailable: "TUTOR_UNAVAILABLE", http.StatusGatewayTimeout: "TUTOR_TIMEOUT"} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("upstream secret and body"))
			}))
			defer server.Close()
			_, err := NewTutor(server.Client(), server.URL, "model", time.Second).Reply(context.Background(), validTutorContext())
			var f *fault.Error
			if !errors.As(err, &f) || f.Code != code || strings.Contains(f.Message, "secret") {
				t.Fatalf("unexpected provider error %v", err)
			}
		})
	}
}

func TestTutorMapsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	_, err := NewTutor(server.Client(), server.URL, "model", 10*time.Millisecond).Reply(context.Background(), validTutorContext())
	var f *fault.Error
	if !errors.As(err, &f) || f.Code != "TUTOR_TIMEOUT" {
		t.Fatalf("expected tutor timeout, got %v", err)
	}
}

func TestTutorLimitsProviderBodyTo64KiB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 64*1024+1)))
	}))
	defer server.Close()
	_, err := NewTutor(server.Client(), server.URL, "model", time.Second).Reply(context.Background(), validTutorContext())
	var f *fault.Error
	if !errors.As(err, &f) || f.Code != "INVALID_TUTOR_RESPONSE" {
		t.Fatalf("expected size-limited response error, got %v", err)
	}
}

func TestTutorReturnsReadinessDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":"stop"}]}`, `{"assistant_text":"Рассуждение сформулировано.","pedagogy":"confirmation","answer_ready":true}`)
	}))
	defer server.Close()
	got, err := NewTutor(server.Client(), server.URL, "model", time.Second).Reply(t.Context(), validTutorContext())
	if err != nil || !got.AnswerReady {
		t.Fatalf("readiness was not parsed: %+v %v", got, err)
	}
}
