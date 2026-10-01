package app

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sync"
	"testing"

	competencyhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/transport/http"
)

const mapCSV = "Ком,Сост,ОР,Тем 1,Уровень ОР,Задание 1,Критерии 1,Задание 2,Критерии 2\n" +
	"К1,С1,ОР1,Тема,Основной,Вопрос 1,\"Критерий, один\nНе менять текст\",Вопрос 2,Критерий 2\n" +
	",,,,Базовый,Вопрос 3,Критерий 3,,\n" +
	",,ОР2,,,Вопрос 4,Критерий 4,,\n"

func upload(f *fixture, path, field, filename, media string, data []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	h.Set("Content-Type", media)
	p, _ := mw.CreatePart(h)
	_, _ = p.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "https://api.example"+path, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	return w
}

func (f *fixture) admin(t *testing.T) *http.Cookie {
	t.Helper()
	access, _, id := f.register(t, "admin@example.edu")
	if _, err := f.pool.Exec(context.Background(), "UPDATE users SET role='admin' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	return access
}

func TestImportAtomicReplacementAndPermissions(t *testing.T) {
	f := newFixture(t)
	student, _, _ := f.register(t, "student@example.edu")
	const path = "/admin/competency-map/import"
	requireCode(t, upload(f, path, "file", "map.csv", "text/csv", []byte(mapCSV), nil), 401, "UNAUTHORIZED")
	requireCode(t, upload(f, path, "file", "map.csv", "text/csv", []byte(mapCSV), student), 403, "FORBIDDEN")
	admin := f.admin(t)
	w := upload(f, path, "file", "map.csv", "text/csv", []byte(mapCSV), admin)
	if w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	var result competencyhttp.ImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Revision != 1 || result.CompetencyCount != 1 || result.ConstituentCount != 1 || result.OutcomeCount != 2 || result.TaskCount != 4 {
		t.Fatalf("counts: %+v", result)
	}
	var criteria string
	if err := f.pool.QueryRow(context.Background(), "SELECT criteria FROM tasks WHERE question='Вопрос 1'").Scan(&criteria); err != nil {
		t.Fatal(err)
	}
	if criteria != "Критерий, один\nНе менять текст" {
		t.Fatal("criteria changed in DB")
	}
	requireCode(t, upload(f, path, "file", "bad.csv", "text/csv", []byte("Ком,Сост,ОР,Задание 1,Критерии 1\nК,С,О,В,\n"), admin), 422, "CSV_INVALID")
	var count, revision int
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM tasks").Scan(&count)
	_ = f.pool.QueryRow(context.Background(), "SELECT revision FROM competency_map_state").Scan(&revision)
	if count != 4 || revision != 1 {
		t.Fatal("failed import changed active database")
	}
	replacement := "Ком,Сост,ОР,Задание 1,Критерии 1\nНовая,С,О,Вопрос,Критерий\n"
	w = upload(f, path, "file", "new.csv", "text/csv", []byte(replacement), admin)
	if w.Code != 200 {
		t.Fatalf("replace: %s", w.Body.String())
	}
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM tasks").Scan(&count)
	_ = f.pool.QueryRow(context.Background(), "SELECT revision FROM competency_map_state").Scan(&revision)
	if count != 1 || revision != 2 {
		t.Fatal("not a complete replacement")
	}
	requireCode(t, upload(f, path, "file", "map.json", "application/json", []byte(mapCSV), admin), 415, "UNSUPPORTED_MEDIA_TYPE")
	requireCode(t, upload(f, path, "wrong", "map.csv", "text/csv", []byte(mapCSV), admin), 422, "VALIDATION_ERROR")
}

func TestImportConcurrentRevisions(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	var wg sync.WaitGroup
	revisions := make(chan int64, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", []byte(mapCSV), admin)
			var result competencyhttp.ImportResult
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			revisions <- result.Revision
		}()
	}
	wg.Wait()
	close(revisions)
	seen := map[int64]bool{}
	for revision := range revisions {
		seen[revision] = true
	}
	if !seen[1] || !seen[2] || len(seen) != 2 {
		t.Fatalf("revision race: %v", seen)
	}
}
