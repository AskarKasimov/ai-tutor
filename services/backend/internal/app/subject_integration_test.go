package app

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSubjectCatalogPermissionsAndEmptySubjects(t *testing.T) {
	f := newFixture(t)
	student, _, _ := f.register(t, "subject-student@example.edu")
	admin := f.admin(t)

	requireCode(t, f.request(http.MethodGet, "/subjects", ""), http.StatusUnauthorized, "UNAUTHORIZED")
	requireCode(t, f.request(http.MethodPost, "/admin/subjects", `{"name":"Пустой предмет"}`), http.StatusUnauthorized, "UNAUTHORIZED")
	requireCode(t, f.request(http.MethodPost, "/admin/subjects", `{"name":"   "}`, admin), http.StatusUnprocessableEntity, "VALIDATION_ERROR")
	studentCatalog := f.request(http.MethodGet, "/subjects", "", student)
	if studentCatalog.Code != http.StatusOK || studentCatalog.Body.String() != "[]\n" {
		t.Fatalf("empty student catalog: %d %s", studentCatalog.Code, studentCatalog.Body.String())
	}
	requireCode(t, f.request(http.MethodPost, "/admin/subjects", `{"name":"Пустой предмет"}`, student), http.StatusForbidden, "FORBIDDEN")

	created := f.request(http.MethodPost, "/admin/subjects", `{"name":"  Пустой предмет  "}`, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create subject: %d %s", created.Code, created.Body.String())
	}
	var value struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Ready bool   `json:"ready"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.ID == "" || value.Name != "Пустой предмет" || value.Ready {
		t.Fatalf("created subject: %+v", value)
	}
	var persistedName string
	if err := f.pool.QueryRow(context.Background(), `SELECT name FROM subjects WHERE id=$1`, value.ID).Scan(&persistedName); err != nil || persistedName != value.Name {
		t.Fatalf("persisted subject name=%q err=%v", persistedName, err)
	}

	adminCatalog := f.request(http.MethodGet, "/subjects", "", admin)
	var items []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Ready bool   `json:"ready"`
	}
	if adminCatalog.Code != http.StatusOK || json.Unmarshal(adminCatalog.Body.Bytes(), &items) != nil {
		t.Fatalf("admin catalog: %d %s", adminCatalog.Code, adminCatalog.Body.String())
	}
	if len(items) != 2 || items[0].Ready || items[1].Ready {
		t.Fatalf("admin should see both empty subjects: %+v", items)
	}
}

func TestSubjectCatalogShowsOnlyVariantgenReadySubjectsToStudents(t *testing.T) {
	f := newFixture(t)
	student, _, _ := f.register(t, "subject-ready-student@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import ML map: %d %s", w.Code, w.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE subjects SET active_revision=(SELECT revision FROM competency_map_state WHERE singleton=true) WHERE id='subject:intro-to-ml'`); err != nil {
		t.Fatal(err)
	}
	created := f.request(http.MethodPost, "/admin/subjects", `{"name":"Пока без карты"}`, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create empty subject: %d %s", created.Code, created.Body.String())
	}

	studentCatalog := f.request(http.MethodGet, "/subjects", "", student)
	var studentItems []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Ready bool   `json:"ready"`
	}
	if studentCatalog.Code != http.StatusOK || json.Unmarshal(studentCatalog.Body.Bytes(), &studentItems) != nil {
		t.Fatalf("student catalog: %d %s", studentCatalog.Code, studentCatalog.Body.String())
	}
	if len(studentItems) != 1 || studentItems[0].ID != "subject:intro-to-ml" || studentItems[0].Name != "Введение в ML" || !studentItems[0].Ready {
		t.Fatalf("student catalog should contain only ready ML: %+v", studentItems)
	}

	adminCatalog := f.request(http.MethodGet, "/subjects", "", admin)
	var adminItems []struct {
		ID    string `json:"id"`
		Ready bool   `json:"ready"`
	}
	if adminCatalog.Code != http.StatusOK || json.Unmarshal(adminCatalog.Body.Bytes(), &adminItems) != nil || len(adminItems) != 2 {
		t.Fatalf("admin catalog: %d %s", adminCatalog.Code, adminCatalog.Body.String())
	}
	readyCount := 0
	for _, item := range adminItems {
		if item.Ready {
			readyCount++
		}
	}
	if readyCount != 1 {
		t.Fatalf("admin readiness values: %+v", adminItems)
	}

	assertStudentReadiness := func(wantReady bool) {
		t.Helper()
		response := f.request(http.MethodGet, "/subjects", "", student)
		var catalog []struct {
			ID string `json:"id"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil {
			t.Fatalf("student catalog after readiness change: %d %s", response.Code, response.Body.String())
		}
		if (len(catalog) == 1) != wantReady {
			t.Fatalf("student readiness want %v, got %v", wantReady, catalog)
		}
	}
	eligibilityChecks := []struct {
		field, invalidate, restore string
	}{
		{"TRUE outcome", `UPDATE outcomes SET include_in_test=false`, `UPDATE outcomes SET include_in_test=true`},
		{"known taxonomy", `UPDATE outcomes SET taxonomy_id=NULL`, `UPDATE outcomes SET taxonomy_id='taxonomy:application'`},
		{"importance range", `UPDATE outcomes SET importance=NULL`, `UPDATE outcomes SET importance=4`},
		{"nonblank question", `UPDATE tasks SET question=chr(9)`, `UPDATE tasks SET question='Вопрос'`},
		{"nonblank voice instruction", `UPDATE tasks SET voice_instruction=NULL`, `UPDATE tasks SET voice_instruction='Назовите ответ.'`},
		{"nonblank reference answer", `UPDATE tasks SET reference_answer=NULL`, `UPDATE tasks SET reference_answer='Ответ'`},
	}
	for _, check := range eligibilityChecks {
		if _, err := f.pool.Exec(context.Background(), check.invalidate); err != nil {
			t.Fatalf("invalidate %s: %v", check.field, err)
		}
		assertStudentReadiness(false)
		if _, err := f.pool.Exec(context.Background(), check.restore); err != nil {
			t.Fatalf("restore %s: %v", check.field, err)
		}
		assertStudentReadiness(true)
	}
}
