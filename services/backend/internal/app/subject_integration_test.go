package app

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSubjectCatalogPermissionsAndEmptySubjects(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM subjects WHERE id='subject:test'`); err != nil {
		t.Fatal(err)
	}
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
	if len(items) != 1 || items[0].Ready {
		t.Fatalf("admin should see the created empty subject: %+v", items)
	}
}

// Without the graph fixture a course is ready only with its own published graph.
func TestSubjectCatalogShowsOnlyReadySubjectsToStudents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	student, _, _ := f.register(t, "subject-ready-student@example.edu")
	admin := f.admin(t)
	created := f.request(http.MethodPost, "/admin/subjects", `{"name":"Пока без материалов"}`, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create empty subject: %d %s", created.Code, created.Body.String())
	}
	setStatus := func(status string) {
		t.Helper()
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO subject_materials(id, subject_id, filename, content_type, s3_raw_key, s3_parsed_key, status, created_at, updated_at)
VALUES ('material:ready', 'subject:test', 'lecture.pdf', 'application/pdf', 'raw', 'parsed', $1, 1, 1)
ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status`, status); err != nil {
			t.Fatal(err)
		}
	}
	studentCatalog := func() []struct {
		ID    string `json:"id"`
		Ready bool   `json:"ready"`
	} {
		t.Helper()
		response := f.request(http.MethodGet, "/subjects", "", student)
		var items []struct {
			ID    string `json:"id"`
			Ready bool   `json:"ready"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &items) != nil {
			t.Fatalf("student catalog: %d %s", response.Code, response.Body.String())
		}
		return items
	}
	if items := studentCatalog(); len(items) != 0 {
		t.Fatalf("subjects without materials are not ready: %+v", items)
	}
	setStatus("failed")
	if items := studentCatalog(); len(items) != 0 {
		t.Fatalf("a failed material does not make a subject ready: %+v", items)
	}
	setStatus("parsed")
	if items := studentCatalog(); len(items) != 0 {
		t.Fatalf("a parsed material without a graph does not make a subject ready: %+v", items)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO course_graphs(subject_id, fingerprint, graph, updated_at) VALUES ('subject:test', 'f', '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	if items := studentCatalog(); len(items) != 1 || items[0].ID != "subject:test" || !items[0].Ready {
		t.Fatalf("student catalog should contain only the subject with a published graph: %+v", items)
	}
	adminCatalog := f.request(http.MethodGet, "/subjects", "", admin)
	var adminItems []struct {
		ID    string `json:"id"`
		Ready bool   `json:"ready"`
	}
	if adminCatalog.Code != http.StatusOK || json.Unmarshal(adminCatalog.Body.Bytes(), &adminItems) != nil || len(adminItems) != 2 {
		t.Fatalf("admin catalog: %d %s", adminCatalog.Code, adminCatalog.Body.String())
	}
}

func TestSubjectWithAGraphOfACompetencyMapIsReadyWithoutMaterials(t *testing.T) {
	t.Parallel()
	f := newFixtureWithConfig(t, testConfig())
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM subjects WHERE id='subject:test'`); err != nil {
		t.Fatal(err)
	}
	student, _, _ := f.register(t, "subject-map-student@example.edu")
	admin := f.admin(t)
	created := f.request(http.MethodPost, "/admin/subjects", `{"name":"Только карта"}`, admin)
	var value struct {
		ID    string `json:"id"`
		Ready bool   `json:"ready"`
	}
	// Without a default graph a new course has nothing to examine by.
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &value) != nil || value.Ready {
		t.Fatalf("a new subject is not ready: %d %s", created.Code, created.Body.String())
	}
	catalog := func() []struct {
		ID    string `json:"id"`
		Ready bool   `json:"ready"`
	} {
		response := f.request(http.MethodGet, "/subjects", "", student)
		var items []struct {
			ID    string `json:"id"`
			Ready bool   `json:"ready"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &items) != nil {
			t.Fatalf("student catalog: %d %s", response.Code, response.Body.String())
		}
		return items
	}
	if items := catalog(); len(items) != 0 {
		t.Fatalf("student catalog before the graph: %+v", items)
	}
	publishGraph(t, f, value.ID, []byte(`{"nodes":[{"id":"a","title":"Агент"}]}`))
	if items := catalog(); len(items) != 1 || items[0].ID != value.ID || !items[0].Ready {
		t.Fatalf("student catalog should contain the subject of the map without materials: %+v", items)
	}
}
