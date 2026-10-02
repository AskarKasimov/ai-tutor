package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	taskgenhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/transport/http"
)

// TestTaskGenerationGroundsOnOutcomeTasks checks the full public flow with a
// real PostgreSQL map and a stubbed LLM provider.
func TestTaskGenerationGroundsOnOutcomeTasks(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "taskgen@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte(mapCSV), admin); w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	outcomeID := ""
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM outcomes WHERE name='ОР1'").Scan(&outcomeID); err != nil {
		t.Fatal(err)
	}
	// The catalog lists the imported topics with their task counts.
	catalog := f.request("GET", "/outcomes", "", access)
	if catalog.Code != 200 {
		t.Fatalf("catalog: %d %s", catalog.Code, catalog.Body.String())
	}
	var list taskgenhttp.OutcomeList
	if err := json.Unmarshal(catalog.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Outcomes) != 2 {
		t.Fatalf("catalog size: %+v", list)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.Messages) != 2 || !strings.Contains(payload.Messages[1].Content, `"outcome":"ОР1"`) || !strings.Contains(payload.Messages[1].Content, `"question":"Вопрос 1"`) {
			t.Errorf("grounding payload omitted samples: %s", payload.Messages)
		}
		response := `{"choices":[{"message":{"content":"{\"tasks\":[{\"question\":\"Новый вопрос 1?\",\"criteria\":\"Критерий\",\"voice_instruction\":\"Ответьте.\"},{\"question\":\"Новый вопрос 2?\",\"criteria\":\"Критерий\",\"voice_instruction\":\"Ответьте.\"}]}"},"finish_reason":"stop"}]}`
		_, _ = w.Write([]byte(response))
	}))
	defer provider.Close()
	f.app.cfg.AssessmentBaseURL = provider.URL
	w := f.request("POST", "/outcomes/"+outcomeID+"/training-tasks", `{"count":2}`, access)
	if w.Code != 200 {
		t.Fatalf("generate: %d %s", w.Code, w.Body.String())
	}
	var result taskgenhttp.GenerateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OutcomeID != outcomeID || len(result.Tasks) != 2 || result.Tasks[0].Question != "Новый вопрос 1?" {
		t.Fatalf("generated set: %+v", result)
	}
	// Unknown outcome and unauthenticated access are rejected.
	requireCode(t, f.request("POST", "/outcomes/does-not-exist/training-tasks", `{"count":1}`, access), 404, "OUTCOME_NOT_FOUND")
	requireCode(t, f.request("POST", "/outcomes/"+outcomeID+"/training-tasks", `{"count":1}`), 401, "UNAUTHORIZED")
}
