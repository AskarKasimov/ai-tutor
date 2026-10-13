package modelapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
)

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
