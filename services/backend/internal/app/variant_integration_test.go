package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	variantgenpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/infrastructure/postgres"
)

func variantMapCSV(t *testing.T) []byte {
	t.Helper()
	rows := [][]string{
		{"", "Компетенция", "Уровень темы", "Составляющая", "Образовательный результат", "Что должно войти в тест", "Таксономия", "Уровень ALDs", "Важность", "Раздел РПД · компетенции РПД", "Задание 1", "ОС"},
		{"", "Регрессия", "базовый", "Линейная модель", "Знает параметры данных", "TRUE", "Знание", "базовый", "5", "Р.1 Введение в анализ данных\nОПК-2", "Экран: Назовите объект наблюдения.\nГолосовая инструкция: Назовите ответ.\nОтвет: строка таблицы", "Нужно знать структуру таблицы."},
		{"", "", "", "", "Понимает целевую переменную", "TRUE", "Понимание", "базовый", "4", "", "Экран: Что предсказывает модель?\nГолосовая инструкция: Назовите ответ.\nОтвет: целевую переменную", "Нужно знать цель модели."},
		{"", "", "", "", "Применяет линейную регрессию", "TRUE", "Применение", "средний", "5", "", "Экран: Вычислите прогноз.\nГолосовая инструкция: Назовите ответ.\nОтвет: сумма признаков с весами", "Нужно знать линейную модель."},
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestVariantCreateReadIdempotencyOwnershipAndHistoricalReader(t *testing.T) {
	f := newFixture(t)
	access, _, ownerID := f.register(t, "variant-owner@example.edu")
	admin := f.admin(t)
	const importPath = "/admin/competency-map/import"
	if w := upload(f, importPath, "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	create := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(""))
		req.Header.Set("Idempotency-Key", key)
		req.AddCookie(access)
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, req)
		return w
	}
	// Concurrent requests with the same idempotency key must converge on one
	// persisted variant even when both reach the insert path together.
	start := make(chan struct{})
	responses := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			responses[i] = create("variant-request-1")
		}(i)
	}
	close(start)
	wg.Wait()
	first := responses[0]
	if first.Code != http.StatusCreated || responses[1].Code != http.StatusCreated {
		t.Fatalf("concurrent create statuses: %d %s; %d %s", first.Code, first.Body.String(), responses[1].Code, responses[1].Body.String())
	}
	var created struct {
		ID           string `json:"id"`
		Competencies []struct {
			Main struct {
				ID           string `json:"id"`
				SourceTaskID string `json:"source_task_id"`
				OutcomeName  string `json:"outcome_name"`
			} `json:"main"`
			Basic []struct {
				ID string `json:"id"`
			} `json:"basic"`
		} `json:"competencies"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.Competencies) != 1 || len(created.Competencies[0].Basic) != 2 {
		t.Fatalf("unexpected created variant: %s", first.Body.String())
	}
	var concurrentReplay struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responses[1].Body.Bytes(), &concurrentReplay); err != nil || concurrentReplay.ID != created.ID {
		t.Fatalf("concurrent requests created different variants: %s / %s", first.Body.String(), responses[1].Body.String())
	}
	mainTaskID, sourceTaskID := created.Competencies[0].Main.ID, created.Competencies[0].Main.SourceTaskID
	replay := create("variant-request-1")
	var replayBody struct {
		ID string `json:"id"`
	}
	if replay.Code != http.StatusCreated || json.Unmarshal(replay.Body.Bytes(), &replayBody) != nil || replayBody.ID != created.ID {
		t.Fatalf("idempotent replay: %d %s", replay.Code, replay.Body.String())
	}
	if w := f.request(http.MethodGet, "/variants/"+created.ID, "", access); w.Code != http.StatusOK {
		t.Fatalf("read: %d %s", w.Code, w.Body.String())
	} else if strings.Contains(w.Body.String(), "строка таблицы") || strings.Contains(w.Body.String(), "Нужно знать структуру") {
		t.Fatal("public variant exposed answer or OS")
	}
	if w := f.request(http.MethodGet, "/variants/"+created.ID+"/tasks/"+mainTaskID, "", access); w.Code != http.StatusOK {
		t.Fatalf("read task: %d %s", w.Code, w.Body.String())
	}
	second := create("variant-request-2")
	if second.Code != http.StatusCreated {
		t.Fatalf("second create: %d %s", second.Code, second.Body.String())
	}
	var secondBody struct {
		ID           string `json:"id"`
		Competencies []struct {
			Main struct {
				ID string `json:"id"`
			} `json:"main"`
		} `json:"competencies"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondBody); err != nil || secondBody.ID == "" || len(secondBody.Competencies) != 1 {
		t.Fatalf("invalid second variant: %s", second.Body.String())
	}
	foreignVariantTaskID := secondBody.Competencies[0].Main.ID
	if w := f.request(http.MethodGet, "/variants/"+created.ID+"/tasks/"+foreignVariantTaskID, "", access); w.Code != http.StatusNotFound {
		t.Fatalf("task from another owned variant status %d", w.Code)
	}
	page1 := f.request(http.MethodGet, "/variants?limit=1", "", access)
	if page1.Code != http.StatusOK {
		t.Fatalf("list first page: %d %s", page1.Code, page1.Body.String())
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(page1.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("invalid first page: %s", page1.Body.String())
	}
	firstPageID := page.Items[0].ID
	cursor := page.NextCursor
	page2 := f.request(http.MethodGet, "/variants?limit=1&cursor="+url.QueryEscape(cursor), "", access)
	if page2.Code != http.StatusOK {
		t.Fatalf("list second page: %d %s", page2.Code, page2.Body.String())
	}
	page = struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}{}
	if err := json.Unmarshal(page2.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].ID == firstPageID || page.NextCursor != "" {
		t.Fatalf("invalid second page after %q (cursor %q): %s", firstPageID, cursor, page2.Body.String())
	}
	if got := f.request(http.MethodGet, fmt.Sprintf("/variants/%s/tasks/no-such-task", created.ID), "", access); got.Code != http.StatusNotFound {
		t.Fatalf("missing task status %d", got.Code)
	}
	other, _, _ := f.register(t, "variant-other@example.edu")
	if w := f.request(http.MethodGet, "/variants/"+created.ID, "", other); w.Code != http.StatusNotFound {
		t.Fatalf("foreign variant status %d", w.Code)
	}
	if w := f.request(http.MethodGet, "/variants/"+created.ID+"/tasks/"+mainTaskID, "", other); w.Code != http.StatusNotFound {
		t.Fatalf("foreign task status %d", w.Code)
	}
	if w := upload(f, importPath, "file", "replacement.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("replacement import: %d %s", w.Code, w.Body.String())
	}
	gradingTask, err := variantgenpg.New(f.pool).TaskForGrading(context.Background(), ownerID, created.ID, mainTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if gradingTask.ID != mainTaskID || gradingTask.Role != "main" {
		t.Fatalf("historical grading position incomplete: %+v", gradingTask)
	}
	profile := gradingTask.Task
	answers := map[string]string{
		"Знает параметры данных":       "строка таблицы",
		"Понимает целевую переменную":  "целевую переменную",
		"Применяет линейную регрессию": "сумма признаков с весами",
	}
	educationalContent := map[string]string{
		"Знает параметры данных":       "Нужно знать структуру таблицы.",
		"Понимает целевую переменную":  "Нужно знать цель модели.",
		"Применяет линейную регрессию": "Нужно знать линейную модель.",
	}
	expectedAnswer, expectedContent := answers[created.Competencies[0].Main.OutcomeName], educationalContent[created.Competencies[0].Main.OutcomeName]
	if profile.ID != sourceTaskID || profile.ReferenceAnswer == nil || *profile.ReferenceAnswer != expectedAnswer || profile.Outcome.EducationalContent == nil || *profile.Outcome.EducationalContent != expectedContent || profile.Outcome.TaxonomyCode == nil || profile.Outcome.ALDLevelCode == nil || profile.Outcome.Importance == nil || profile.Outcome.IncludeInTest == nil || !*profile.Outcome.IncludeInTest || len(profile.Constituent.Sections) != 1 || len(profile.Constituent.Sections[0].CurriculumCompetencies) != 1 {
		t.Fatalf("historical grading snapshot incomplete: %+v", profile)
	}
}

func TestVariantCreationRollsBackWhenSnapshotInsertFails(t *testing.T) {
	f := newFixture(t)
	access, _, ownerID := f.register(t, "variant-rollback@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	_, err := f.pool.Exec(context.Background(), `
CREATE FUNCTION fail_variant_task_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'forced snapshot failure'; END;
$$;`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(context.Background(), `CREATE TRIGGER fail_variant_task_insert BEFORE INSERT ON variant_tasks FOR EACH ROW EXECUTE FUNCTION fail_variant_task_insert()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_variant_task_insert ON variant_tasks`)
		_, _ = f.pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS fail_variant_task_insert()`)
	})
	req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(""))
	req.Header.Set("Idempotency-Key", "variant-rollback-request")
	req.AddCookie(access)
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected failed transaction, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM variants WHERE user_id = $1`, ownerID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed variant creation left %d parent rows", count)
	}
}
