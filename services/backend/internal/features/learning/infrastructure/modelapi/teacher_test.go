package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
)

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
				_, err = teacher.Lesson(context.Background(), learning.LessonInput{})
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
			if requests.Load() != int32(wantRequests) {
				t.Fatalf("requests = %d, want %d", requests.Load(), wantRequests)
			}
		})
	}
}

func TestLessonDiscardsPartialResultWhenPracticeFails(t *testing.T) {
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
			result, err := New(server.Client(), server.URL, "test-model", time.Second).Lesson(t.Context(), learning.LessonInput{})
			if err == nil || result != (learning.Lesson{}) || requests.Load() != int32(failAt) {
				t.Fatalf("partial lesson escaped: result=%+v, err=%v, requests=%d", result, err, requests.Load())
			}
		})
	}
}

func TestTeacherPreservesMarkdownInJSONTextFields(t *testing.T) {
	theory := "**Обновление весов**\n\n```python\nfor step in range(2):\n    print(\"a\\nb\")\n```\n\n| Шаг | Loss |\n|---|---:|\n| 1 | 0.5 |"
	theory += "\n\n$$\n\\frac{a+b}{2}\n$$"
	question := "Объясните $\\hat{y}$."
	feedback := "**Уточните:** вычитайте $\\nabla L$."
	for _, check := range []bool{false, true} {
		name := "lesson"
		if check {
			name = "check"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ResponseFormat struct {
						Type string `json:"type"`
					} `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.ResponseFormat.Type != "json_object" {
					t.Errorf("response mode = %q", request.ResponseFormat.Type)
				}
				result := map[string]any{"theory": theory, "question": question}
				if check {
					result = map[string]any{"correct": false, "feedback": feedback, "question": question}
				}
				content, err := json.Marshal(result)
				if err != nil {
					t.Error(err)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}}})
			}))
			defer server.Close()
			teacher := New(server.Client(), server.URL, "teacher", time.Second)
			if check {
				result, err := teacher.Check(context.Background(), learning.CheckInput{})
				if err != nil {
					t.Fatal(err)
				}
				if result.Correct || result.Feedback != feedback || result.Question != question {
					t.Fatalf("check = %+v", result)
				}
			} else {
				result, err := teacher.Lesson(context.Background(), learning.LessonInput{})
				if err != nil {
					t.Fatal(err)
				}
				if result.Theory != theory || result.Question != question {
					t.Fatalf("lesson = %+v", result)
				}
			}
		})
	}
}
