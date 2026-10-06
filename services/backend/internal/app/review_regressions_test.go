package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaterialHTTPAcceptsMaximumContentAndRejectsOversize(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	response := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte("Ком,Сост,ОР,Задание 1,Критерии 1\nК,С,О,В,К\n"), admin)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var id string
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM outcomes").Scan(&id); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"name": "notes", "content": strings.Repeat("я", 100000), "outcome_ids": []string{id}})
	response = f.request("POST", "/admin/materials", string(body), admin)
	if response.Code != 201 {
		t.Fatalf("valid material: %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM material_chunks").Scan(&count); err != nil || count == 0 {
		t.Fatalf("not persisted: %d %v", count, err)
	}
	// Six-byte JSON escapes are still valid input for the same 100,000 characters.
	escaped := strings.ReplaceAll(string(body), "я", `\u044f`)
	response = f.request("POST", "/admin/materials", escaped, admin)
	if response.Code != 201 {
		t.Fatalf("escaped material: %d %s", response.Code, response.Body.String())
	}
	emojiBody, _ := json.Marshal(map[string]any{"name": "emoji", "content": strings.Repeat("😀", 100000), "outcome_ids": []string{id}})
	escapedEmoji := strings.ReplaceAll(string(emojiBody), "😀", `\ud83d\ude00`)
	response = f.request("POST", "/admin/materials", escapedEmoji, admin)
	if response.Code != 201 {
		t.Fatalf("surrogate-pair material: %d %s", response.Code, response.Body.String())
	}
	excessiveContent, _ := json.Marshal(map[string]any{"name": "oversize", "content": strings.Repeat("я", 100001), "outcome_ids": []string{id}})
	response = f.request("POST", "/admin/materials", string(excessiveContent), admin)
	if response.Code != 422 {
		t.Fatalf("excessive content: %d %s", response.Code, response.Body.String())
	}
	response = f.request("POST", "/admin/materials", strings.Repeat(" ", 1400000), admin)
	if response.Code != 413 {
		t.Fatalf("oversize body: %d", response.Code)
	}
}
func TestTaskSearchImportanceRange(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	csv := "Ком,Сост,ОР,Важность,Задание 1,Критерии 1\nК,С,О1,1,Низкий,К\n,,О3,3,Средний,К\n,,О5,5,Высокий,К\n,,Нет важности,,Неизвестный,К\n"
	response := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte(csv), admin)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	for _, tc := range []struct {
		query string
		count int
	}{{"importance_min=2&importance_max=4", 1}, {"importance_min=3", 2}, {"importance_max=1", 1}, {"importance=3", 1}, {"importance=3&importance_min=4", 0}, {"", 4}} {
		response = f.request("GET", "/tasks?"+tc.query, "", admin)
		var tasks []struct {
			Question string `json:"question"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &tasks); err != nil || response.Code != 200 || len(tasks) != tc.count {
			t.Fatalf("%s: %d %s", tc.query, response.Code, response.Body.String())
		}
	}
}
func TestGenerationUsesIndependentModelConfiguration(t *testing.T) {
	f := newFixture(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "generation-only" {
			t.Errorf("wrong model: %q", body.Model)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"question":"Вопрос","options":[],"voice_instruction":"Объясните","reference_answer":"Ответ","criteria":""}`}}}})
	}))
	defer provider.Close()
	configuredEnv(t)
	t.Setenv("BACKEND_TASKGEN_BASE_URL", provider.URL)
	t.Setenv("BACKEND_TASKGEN_MODEL", "generation-only")
	t.Setenv("BACKEND_TASKGEN_TIMEOUT", "5s")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	f.app.cfg = cfg
	admin := f.admin(t)
	response := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte("Ком,Сост,ОР,Задание 1,Критерии 1\nК,С,О,В,К\n"), admin)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var id string
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM outcomes").Scan(&id); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"outcome_id": id})
	r := httptest.NewRequest("POST", "/tasks/generate", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "independent-model")
	r.AddCookie(admin)
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("generation: %d %s", w.Code, w.Body.String())
	}
	var model string
	if err := f.pool.QueryRow(context.Background(), "SELECT model FROM generation_runs").Scan(&model); err != nil || model != "generation-only" {
		t.Fatalf("persisted model %q: %v", model, err)
	}
}

func TestPairedImportPersistsPartialProfileAndSourceLinks(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	csv := "Ком,Сост,ОР,Что должно войти в тест,Таксономия,Важность,Задание 1,Критерии 1\nК,С,О,TRUE,Знание,3,В,К\n,,,,,,В2,К2\n,,Без задания,FALSE,,,,\n"
	response := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte(csv), admin)
	if response.Code != 200 {
		t.Fatalf("partial profile: %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM outcome_source_rows").Scan(&count); err != nil || count != 3 {
		t.Fatalf("source links=%d err=%v", count, err)
	}
	response = f.request("GET", "/tasks?importance=3&include_in_test=true&taxonomy=knowledge", "", admin)
	var tasks []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &tasks); err != nil || len(tasks) != 2 {
		t.Fatalf("filtered profile: %d %s", response.Code, response.Body.String())
	}
	response = f.request("GET", "/tasks/"+tasks[0].ID, "", admin)
	var profile struct {
		Importance *int    `json:"importance"`
		ALD        *string `json:"ald_level_code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil || profile.Importance == nil || *profile.Importance != 3 || profile.ALD != nil {
		t.Fatalf("profile: %s", response.Body.String())
	}
}
