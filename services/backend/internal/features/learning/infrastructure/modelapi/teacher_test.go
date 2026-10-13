package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
)

func TestCheckGradesBeforeGeneratingAndReviewingNewCase(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages  []message `json:"messages"`
			MaxTokens int       `json:"max_tokens"`
			Reasoning string    `json:"reasoning_effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.MaxTokens != 4096 || request.Reasoning != "medium" {
			t.Errorf("output budget: %+v", request)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch calls.Add(1) {
		case 1:
			if payload["question"] != "Старый случай" || strings.Contains(request.Messages[0].Content, "ПРАКТИКА НА ПОНИМАНИЕ") {
				t.Error("grading diluted by question generation")
			}
			result = map[string]any{"correct": false, "feedback": "Свяжите вывод с условиями случая.", "question": ""}
		case 2:
			if payload["previous_question"] != "Старый случай" || payload["theory"] != "Изученная теория" {
				t.Errorf("followup lost context: %+v", payload)
			}
			result = map[string]any{"question": "Черновик другого случая"}
		case 3:
			if payload["previous_question"] != "Старый случай" || payload["draft_question"] != "Черновик другого случая" {
				t.Errorf("review lost previous case: %+v", payload)
			}
			result = map[string]any{"question": "Другой случай с изменённым существенным условием"}
		default:
			t.Error("unexpected extra request")
		}
		content, _ := json.Marshal(result)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	got, err := New(server.Client(), server.URL, "model", time.Second).Check(t.Context(), learning.CheckInput{Theory: "Изученная теория", Question: "Старый случай", Answer: "Общий пересказ"})
	if err != nil || got.Correct || got.Question != "Другой случай с изменённым существенным условием" || calls.Load() != 3 {
		t.Fatalf("result: %+v, %v, calls=%d", got, err, calls.Load())
	}
}

func TestTeacherRejectsTruncatedCompletionEvenWithValidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"theory":"Теория"}`}, "finish_reason": "length"}}})
	}))
	defer server.Close()
	if _, err := New(server.Client(), server.URL, "model", time.Second).Theory(t.Context(), learning.LessonInput{}); err == nil {
		t.Fatal("truncated completion accepted")
	}
}

// Exercise the provider boundary, including questions after a wrong answer:
// malformed model output must never become a lesson or a follow-up task.
func TestTeacherResponses(t *testing.T) {
	for _, tc := range []struct {
		name, response         string
		check, last, wantError bool
	}{
		{"lesson", `{"theory":"Теория", "question":"Объясните ошибку в ситуации."}`, false, false, false},
		{"empty lesson question", `{"theory":"Теория", "question":" "}`, false, false, true},
		{"empty theory", `{"theory":" ", "question":"Вопрос"}`, false, false, true},
		{"retry", `{"correct":false,"feedback":"Разбор","question":"Объясните последствие другого действия."}`, true, false, false},
		{"missing retry", `{"correct":false,"feedback":"Разбор","question":""}`, true, false, true},
		{"last attempt", `{"correct":false,"feedback":"Разбор","question":""}`, true, true, false},
		{"correct", `{"correct":true,"feedback":"Верно","question":""}`, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestNumber := requests.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var request struct {
					Messages []message `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
					t.Errorf("invalid request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if !tc.check && requestNumber == 2 {
					var payload struct {
						Theory string `json:"theory"`
					}
					if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil || payload.Theory != "Теория" {
						t.Errorf("practice must use exactly the generated theory: %+v, %v", payload, err)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"message": map[string]any{"content": tc.response},
				}}})
			}))
			defer server.Close()
			teacher := New(server.Client(), server.URL+"/v1", "test-model", time.Second)
			var err error
			if tc.check {
				var result learning.Check
				result, err = teacher.Check(context.Background(), learning.CheckInput{LastAttempt: tc.last})
				if !tc.wantError && (result.Correct || tc.last) && result.Question != "" {
					t.Fatal("unexpected next question after completion")
				}
			} else {
				var theory string
				theory, err = teacher.Theory(context.Background(), learning.LessonInput{})
				if err == nil {
					_, err = teacher.Practice(context.Background(), learning.LessonInput{}, theory)
				}
			}
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error = %v", err, tc.wantError)
			}
			wantRequests := 1
			if !tc.check && tc.name != "empty theory" {
				wantRequests = 2
			}
			if tc.name == "lesson" {
				wantRequests = 3
			}
			if tc.name == "retry" {
				wantRequests = 2
			}
			if tc.name == "missing retry" {
				wantRequests = 2
			}
			if requests.Load() != int32(wantRequests) {
				t.Fatalf("requests = %d, want %d", requests.Load(), wantRequests)
			}
		})
	}
}

func TestPracticeFailureLeavesTheoryAvailable(t *testing.T) {
	for _, failAt := range []int{2, 3} {
		t.Run(strconv.Itoa(failAt), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestNumber := requests.Add(1)
				if requestNumber == int32(failAt) {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				content := `{"theory":"Сохранённая теория"}`
				if requestNumber > 1 {
					content = `{"question":"Черновой вопрос"}`
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
			}))
			defer server.Close()
			teacher := New(server.Client(), server.URL, "test-model", time.Second)
			theory, err := teacher.Theory(t.Context(), learning.LessonInput{})
			if err != nil || theory != "Сохранённая теория" {
				t.Fatalf("theory = %q, error = %v", theory, err)
			}
			question, err := teacher.Practice(t.Context(), learning.LessonInput{}, theory)
			if err == nil || question != "" || requests.Load() != int32(failAt) {
				t.Fatalf("partial practice escaped: question=%q, err=%v, requests=%d", question, err, requests.Load())
			}
		})
	}
}
