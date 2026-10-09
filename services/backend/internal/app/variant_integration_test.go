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
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
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
	const importPath = "/admin/subjects/subject:test/competency-map/import"
	if w := upload(f, importPath, "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	create := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:test"}`))
		req.Header.Set("Content-Type", "application/json")
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
	page1 := f.request(http.MethodGet, "/variants?subject_id=subject%3Atest&limit=1", "", access)
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
	page2 := f.request(http.MethodGet, "/variants?subject_id=subject%3Atest&limit=1&cursor="+url.QueryEscape(cursor), "", access)
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
	if profile.AudioAssetID == nil || *profile.AudioAssetID != "taskaudio_"+profile.ID {
		t.Fatalf("historical task audio asset link missing: %#v", profile.AudioAssetID)
	}
	var savedAudioID *string
	if err := f.pool.QueryRow(context.Background(), `SELECT audio_asset_id FROM variant_tasks WHERE id=$1`, mainTaskID).Scan(&savedAudioID); err != nil || savedAudioID == nil || *savedAudioID != *profile.AudioAssetID {
		t.Fatalf("variant task did not retain audio link: id=%v err=%v", savedAudioID, err)
	}
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

func TestVariantsAreScopedBySubjectAndRetainSubjectSnapshot(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "subject-variant-owner@example.edu")
	admin := f.admin(t)
	createdSubject := f.request(http.MethodPost, "/admin/subjects", `{"name":"Физика"}`, admin)
	if createdSubject.Code != http.StatusCreated {
		t.Fatalf("create B: %d %s", createdSubject.Code, createdSubject.Body.String())
	}
	var subjectB struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createdSubject.Body.Bytes(), &subjectB); err != nil || subjectB.ID == "" {
		t.Fatalf("created subject B: %s (%v)", createdSubject.Body.String(), err)
	}
	const subjectA = "subject:test"
	mapA := variantMapCSV(t)
	mapB := []byte(strings.ReplaceAll(string(mapA), "Применяет линейную регрессию", "Применяет закон Ома"))
	if response := upload(f, "/admin/subjects/"+subjectA+"/competency-map/import", "file", "a.csv", "text/csv", mapA, admin); response.Code != http.StatusOK {
		t.Fatalf("import A: %d %s", response.Code, response.Body.String())
	}
	if response := upload(f, "/admin/subjects/"+subjectB.ID+"/competency-map/import", "file", "b.csv", "text/csv", mapB, admin); response.Code != http.StatusOK {
		t.Fatalf("import B: %d %s", response.Code, response.Body.String())
	}
	create := func(key, subjectID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"`+subjectID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		req.AddCookie(access)
		w := httptest.NewRecorder()
		f.app.Handler().ServeHTTP(w, req)
		return w
	}
	if missing := f.request(http.MethodPost, "/variants", `{}`, access); missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing subject_id status %d: %s", missing.Code, missing.Body.String())
	}
	if unknown := create("subject-unknown", "subject:missing"); unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown subject status %d: %s", unknown.Code, unknown.Body.String())
	}
	emptySubject := f.request(http.MethodPost, "/admin/subjects", `{"name":"Пустой предмет"}`, admin)
	var empty struct {
		ID string `json:"id"`
	}
	if emptySubject.Code != http.StatusCreated || json.Unmarshal(emptySubject.Body.Bytes(), &empty) != nil || empty.ID == "" {
		t.Fatalf("create empty subject: %d %s", emptySubject.Code, emptySubject.Body.String())
	}
	if unavailable := create("subject-empty", empty.ID); unavailable.Code != http.StatusConflict || !strings.Contains(unavailable.Body.String(), "NO_ELIGIBLE_COMPETENCIES") {
		t.Fatalf("empty subject status %d: %s", unavailable.Code, unavailable.Body.String())
	}
	variantA := create("subject-a", subjectA)
	if variantA.Code != http.StatusCreated || !strings.Contains(variantA.Body.String(), "Применяет линейную регрессию") || strings.Contains(variantA.Body.String(), "Применяет закон Ома") {
		t.Fatalf("create A used wrong map: %d %s", variantA.Code, variantA.Body.String())
	}
	variantB := create("subject-b", subjectB.ID)
	if variantB.Code != http.StatusCreated || !strings.Contains(variantB.Body.String(), "Применяет закон Ома") || strings.Contains(variantB.Body.String(), "Применяет линейную регрессию") {
		t.Fatalf("create B used wrong map: %d %s", variantB.Code, variantB.Body.String())
	}
	if conflict := create("subject-a", subjectB.ID); conflict.Code != http.StatusConflict {
		t.Fatalf("cross-subject key replay status %d: %s", conflict.Code, conflict.Body.String())
	}
	var createdA struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(variantA.Body.Bytes(), &createdA); err != nil {
		t.Fatal(err)
	}
	mapANew := []byte(strings.ReplaceAll(string(mapA), "Применяет линейную регрессию", "Обновлённая регрессия"))
	if response := upload(f, "/admin/subjects/"+subjectA+"/competency-map/import", "file", "a2.csv", "text/csv", mapANew, admin); response.Code != http.StatusOK {
		t.Fatalf("reimport A: %d %s", response.Code, response.Body.String())
	}
	oldVariant := f.request(http.MethodGet, "/variants/"+createdA.ID, "", access)
	if oldVariant.Code != http.StatusOK || !strings.Contains(oldVariant.Body.String(), "Применяет линейную регрессию") || !strings.Contains(oldVariant.Body.String(), "subject:test") || !strings.Contains(oldVariant.Body.String(), "Тестовый предмет") {
		t.Fatalf("historical variant lost its subject/map snapshot: %d %s", oldVariant.Code, oldVariant.Body.String())
	}
	if missing := f.request(http.MethodGet, "/variants", "", access); missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("list without subject_id status %d: %s", missing.Code, missing.Body.String())
	}
	listA := f.request(http.MethodGet, "/variants?subject_id="+url.QueryEscape(subjectA), "", access)
	listB := f.request(http.MethodGet, "/variants?subject_id="+url.QueryEscape(subjectB.ID), "", access)
	for subjectID, response := range map[string]*httptest.ResponseRecorder{subjectA: listA, subjectB.ID: listB} {
		var page struct {
			Items []struct {
				SubjectID string `json:"subject_id"`
			} `json:"items"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].SubjectID != subjectID {
			t.Fatalf("subject history %s: %d %s", subjectID, response.Code, response.Body.String())
		}
	}
}

func TestVariantSnapshotLockBlocksConcurrentSubjectImport(t *testing.T) {
	f := newFixture(t)
	_, _, ownerID := f.register(t, "variant-lock-owner@example.edu")
	admin := f.admin(t)
	if response := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "a.csv", "text/csv", variantMapCSV(t), admin); response.Code != http.StatusOK {
		t.Fatalf("import A: %d %s", response.Code, response.Body.String())
	}
	var revisionBefore int64
	if err := f.pool.QueryRow(context.Background(), `SELECT active_revision FROM subjects WHERE id='subject:test'`).Scan(&revisionBefore); err != nil {
		t.Fatal(err)
	}
	buildStarted, continueBuild := make(chan struct{}), make(chan struct{})
	created := make(chan error, 1)
	go func() {
		_, err := variantgenpg.New(f.pool).Create(context.Background(), ownerID, "subject:test", "variant-lock-test",
			func(subjectID, subjectName string, revision int64, candidates []variant.CandidateOutcome) (variant.Variant, error) {
				close(buildStarted)
				<-continueBuild
				return variant.Variant{ID: "variant-lock-test", SubjectID: subjectID, SubjectNameSnapshot: subjectName,
					MapRevision: revision, AlgorithmVersion: "test", IncludedCompetencyCount: 1,
					SkippedCompetencies: []variant.SkippedCompetency{}, CreatedAt: 1}, nil
			})
		created <- err
	}()
	<-buildStarted
	importStarted, importDone := make(chan struct{}), make(chan *httptest.ResponseRecorder, 1)
	go func() {
		close(importStarted)
		importDone <- upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "a2.csv", "text/csv", variantMapCSV(t), admin)
	}()
	<-importStarted
	select {
	case response := <-importDone:
		t.Fatalf("subject import passed the held variant snapshot lock: %d %s", response.Code, response.Body.String())
	case <-time.After(100 * time.Millisecond):
	}
	close(continueBuild)
	if err := <-created; err != nil {
		t.Fatalf("create variant: %v", err)
	}
	select {
	case response := <-importDone:
		if response.Code != http.StatusOK {
			t.Fatalf("reimport A: %d %s", response.Code, response.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subject import did not finish after variant snapshot committed")
	}
	var savedRevision, activeRevision int64
	if err := f.pool.QueryRow(context.Background(), `SELECT map_revision FROM variants WHERE id='variant-lock-test'`).Scan(&savedRevision); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT active_revision FROM subjects WHERE id='subject:test'`).Scan(&activeRevision); err != nil {
		t.Fatal(err)
	}
	if savedRevision != revisionBefore || activeRevision <= savedRevision {
		t.Fatalf("variant/import revisions saved=%d active=%d before=%d", savedRevision, activeRevision, revisionBefore)
	}
}

func TestVariantCreationRollsBackWhenSnapshotInsertFails(t *testing.T) {
	f := newFixture(t)
	access, _, ownerID := f.register(t, "variant-rollback@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
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
	req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:test"}`))
	req.Header.Set("Content-Type", "application/json")
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

func TestVariableSizeVariantsPersistReadAndListActualTaskCount(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "variable-variant@example.edu")
	admin := f.admin(t)
	rows, err := csv.NewReader(bytes.NewReader(variantMapCSV(t))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	mixed := [][]string{rows[0]}
	for size := 1; size <= 3; size++ {
		for i := 4 - size; i < 4; i++ {
			row := append([]string(nil), rows[i]...)
			if i == 4-size {
				row[1], row[2], row[3] = fmt.Sprintf("Компетенция %d", size), "базовый", "Модель"
			}
			mixed = append(mixed, row)
		}
	}
	var content bytes.Buffer
	writer := csv.NewWriter(&content)
	if err := writer.WriteAll(mixed); err != nil {
		t.Fatal(err)
	}
	if w := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", content.Bytes(), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %s", w.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "variable-variant")
	req.AddCookie(access)
	response := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var value struct {
		ID           string `json:"id"`
		TaskCount    int    `json:"task_count"`
		Competencies []struct {
			Basic []json.RawMessage `json:"basic"`
		} `json:"competencies"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.TaskCount != 6 || len(value.Competencies) != 3 {
		t.Fatalf("variant: %s", response.Body.String())
	}
	for i, block := range value.Competencies {
		if block.Basic == nil || len(block.Basic) != i {
			t.Fatalf("block %d: %s", i, response.Body.String())
		}
	}
	read := f.request(http.MethodGet, "/variants/"+value.ID, "", access)
	if read.Code != http.StatusOK {
		t.Fatalf("read: %s", read.Body.String())
	}
	if err := json.Unmarshal(read.Body.Bytes(), &value); err != nil || value.TaskCount != 6 || len(value.Competencies) != 3 {
		t.Fatalf("saved variant: %s / %v", read.Body.String(), err)
	}
	for i, block := range value.Competencies {
		if block.Basic == nil || len(block.Basic) != i {
			t.Fatalf("saved block %d: %s", i, read.Body.String())
		}
	}
	list := f.request(http.MethodGet, "/variants?subject_id=subject%3Atest", "", access)
	var page struct {
		Items []struct {
			TaskCount int `json:"task_count"`
		} `json:"items"`
	}
	if list.Code != http.StatusOK || json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].TaskCount != 6 {
		t.Fatalf("history: %s", list.Body.String())
	}
}
