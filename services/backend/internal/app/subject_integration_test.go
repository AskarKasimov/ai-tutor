package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSubjectMapRoutesRequireAuthenticationAndAdminImport(t *testing.T) {
	f := newFixture(t)
	student, _, _ := f.register(t, "subject-map-student@example.edu")
	admin := f.admin(t)
	const path = "/admin/subjects/subject:intro-to-ml/competency-map/import"
	requireCode(t, upload(f, path, "file", "map.csv", "text/csv", variantMapCSV(t), nil), http.StatusUnauthorized, "UNAUTHORIZED")
	requireCode(t, upload(f, path, "file", "map.csv", "text/csv", variantMapCSV(t), student), http.StatusForbidden, "FORBIDDEN")
	if response := upload(f, path, "file", "map.csv", "text/csv", variantMapCSV(t), admin); response.Code != http.StatusOK {
		t.Fatalf("admin subject import: %d %s", response.Code, response.Body.String())
	}
	requireCode(t, f.request(http.MethodGet, "/subjects/subject:intro-to-ml/competency-map", ""), http.StatusUnauthorized, "UNAUTHORIZED")
	if response := f.request(http.MethodGet, "/subjects/subject:intro-to-ml/competency-map", "", student); response.Code != http.StatusOK {
		t.Fatalf("student subject read: %d %s", response.Code, response.Body.String())
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

func TestSubjectImportsReplaceOnlySelectedMapAndKeepSnapshots(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	const mlID = "subject:intro-to-ml"
	created := f.request(http.MethodPost, "/admin/subjects", `{"name":"Вторая дисциплина"}`, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create second subject: %d %s", created.Code, created.Body.String())
	}
	var secondSubject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &secondSubject); err != nil || secondSubject.ID == "" {
		t.Fatalf("created subject: %s (%v)", created.Body.String(), err)
	}
	const subjectImport = "/admin/subjects/%s/competency-map/import"
	importSubject := func(id, name string, data []byte) *httptest.ResponseRecorder {
		return upload(f, fmt.Sprintf(subjectImport, id), "file", name, "text/csv", data, admin)
	}
	firstA := importSubject(mlID, "a.csv", variantMapCSV(t))
	if firstA.Code != http.StatusOK {
		t.Fatalf("import A: %d %s", firstA.Code, firstA.Body.String())
	}
	mapB := strings.ReplaceAll(string(variantMapCSV(t)), "Регрессия", "Оптимизация B")
	mapB = strings.ReplaceAll(mapB, "Линейная модель", "B model")
	if response := importSubject(secondSubject.ID, "b.csv", []byte(mapB)); response.Code != http.StatusOK {
		t.Fatalf("import B: %d %s", response.Code, response.Body.String())
	}
	var revisionA, revisionB int64
	if err := f.pool.QueryRow(context.Background(), `SELECT active_revision FROM subjects WHERE id=$1`, mlID).Scan(&revisionA); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT active_revision FROM subjects WHERE id=$1`, secondSubject.ID).Scan(&revisionB); err != nil {
		t.Fatal(err)
	}
	if revisionA == 0 || revisionB <= revisionA {
		t.Fatalf("subject revisions A=%d B=%d", revisionA, revisionB)
	}

	readSubject := func(id string) *httptest.ResponseRecorder {
		return f.request(http.MethodGet, fmt.Sprintf("/subjects/%s/competency-map", id), "", admin)
	}
	var readB map[string]any
	before := readSubject(secondSubject.ID)
	if before.Code != http.StatusOK || json.Unmarshal(before.Body.Bytes(), &readB) != nil || !strings.Contains(before.Body.String(), "Оптимизация B") {
		t.Fatalf("read B: %d %s", before.Code, before.Body.String())
	}
	if unknown := readSubject("subject:missing"); unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown subject read status %d: %s", unknown.Code, unknown.Body.String())
	}
	if unknown := importSubject("subject:missing", "missing.csv", variantMapCSV(t)); unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown subject import status %d: %s", unknown.Code, unknown.Body.String())
	}

	var taskID, audioID, outcomeID string
	if err := f.pool.QueryRow(context.Background(), `SELECT task.id, task.audio_asset_id, outcome.id
FROM competencies competency JOIN constituents constituent ON constituent.competency_id=competency.id
JOIN outcomes outcome ON outcome.constituent_id=constituent.id JOIN tasks task ON task.outcome_id=outcome.id
WHERE competency.revision=$1 ORDER BY task.id LIMIT 1`, revisionB).Scan(&taskID, &audioID, &outcomeID); err != nil {
		t.Fatal(err)
	}
	if audioID == "" {
		t.Fatal("B task has no pending audio asset")
	}
	var ownerID string
	if err := f.pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email='admin@example.edu'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO variants(id,user_id,create_request_key,map_revision,algorithm_version,included_competency_count,skipped_competencies,created_at,subject_id,subject_name_snapshot)
VALUES ('variant-b-snapshot',$1,'b-snapshot',$2,'test',1,'[]',1,$3,'Вторая дисциплина')`, ownerID, revisionB, secondSubject.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO variant_tasks(id,variant_id,live_task_id,source_task_id_snapshot,audio_asset_id,competency_position,slot,role,task_snapshot,profile_snapshot)
VALUES ('variant-b-task','variant-b-snapshot',$1,$1,$2,1,0,'main','{"question":"Исторический B snapshot"}','{}')`, taskID, audioID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO material_chunks(id,material_name,ordinal,content,created_at) VALUES ('material-b','B source',1,'Содержание предмета B',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO material_chunk_outcomes(chunk_id,outcome_id) VALUES ('material-b',$1)`, outcomeID); err != nil {
		t.Fatal(err)
	}
	mapANew := strings.ReplaceAll(string(variantMapCSV(t)), "Регрессия", "Замена A")
	if response := importSubject(mlID, "a-replacement.csv", []byte(mapANew)); response.Code != http.StatusOK {
		t.Fatalf("reimport A: %d %s", response.Code, response.Body.String())
	}
	var revisionANew int64
	if err := f.pool.QueryRow(context.Background(), `SELECT active_revision FROM subjects WHERE id=$1`, mlID).Scan(&revisionANew); err != nil || revisionANew <= revisionB {
		t.Fatalf("A revision did not advance globally: A=%d B=%d err=%v", revisionANew, revisionB, err)
	}
	var bRevisionAfter int64
	if err := f.pool.QueryRow(context.Background(), `SELECT active_revision FROM subjects WHERE id=$1`, secondSubject.ID).Scan(&bRevisionAfter); err != nil || bRevisionAfter != revisionB {
		t.Fatalf("B active revision changed: got %d want %d err=%v", bRevisionAfter, revisionB, err)
	}
	after := readSubject(secondSubject.ID)
	if after.Code != http.StatusOK || !strings.Contains(after.Body.String(), "Оптимизация B") {
		t.Fatalf("B map changed after A reimport: %d %s", after.Code, after.Body.String())
	}
	var taskCount, variantCount, materialCount int
	var snapshot string
	var audioStatus string
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM tasks WHERE id=$1`, taskID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM variants WHERE id='variant-b-snapshot' AND subject_id=$1`, secondSubject.ID).Scan(&variantCount); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM material_chunks WHERE id='material-b'`).Scan(&materialCount); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT task_snapshot->>'question' FROM variant_tasks WHERE id='variant-b-task'`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM audio_assets WHERE id=$1`, audioID).Scan(&audioStatus); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || variantCount != 1 || materialCount != 1 || snapshot != "Исторический B snapshot" || audioStatus != "pending" {
		t.Fatalf("A reimport damaged B data: tasks=%d variants=%d materials=%d snapshot=%q audio=%q", taskCount, variantCount, materialCount, snapshot, audioStatus)
	}
}
