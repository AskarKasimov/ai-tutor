package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mapBook is a competency map book of one sheet in the course template:
// one competency with three outcomes for the test, so its graph has a main
// task and two basic ones.
func mapBook(t *testing.T) []byte {
	t.Helper()
	rows := [][]string{
		{"№", "Субкомпетенция (Proficiency)", "Составляющая (Observe)", "Образовательный результат (Observe Behaviour)",
			"Уровень по Блуму", "Важность (1–5)", "Что должно войти в тест", "Задание (устная инструкция)", "Критерии оценивания"},
		{"1", "К1. Агент", "1.1 Описывает агента", "Называет части агента", "Помнить", "4", "TRUE", "Назовите части агента.", "Эталон: сенсоры и исполнительные механизмы."},
		{"2", "", "", "Объясняет рациональность", "Понимать", "3", "TRUE", "Объясните, что такое рациональный агент.", "Эталон: максимизирует меру эффективности."},
		{"3", "", "", "Выбирает тип агента", "Анализировать", "5", "TRUE", "Какой агент нужен роботу-уборщику и почему?", "Эталон: агент с моделью мира.\nБалл (макс. 1): 1 — тип и причина."},
	}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, value := range row {
			var text bytes.Buffer
			if err := xml.EscapeText(&text, []byte(value)); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t>%s</t></is></c>`, 'A'+j, i+1, text.String())
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	parts := map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Карта компетенций" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": sheet.String(),
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportMapAnswersTooLargeForAnOversizedBody(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	admin := f.admin(t)
	big := bytes.Repeat([]byte{'x'}, 27<<20)
	w := upload(f, "/admin/subjects/subject:test/graph/competency-map", "file", "map.xlsx", "application/octet-stream", big, admin)
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("27 MiB map: %d %s", w.Code, w.Body.String())
	}
}

func TestImportMapStartsForSharedCourseWithoutMaterials(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	admin := f.admin(t)
	created := f.request(http.MethodPost, "/admin/subjects", `{"name":"Курс только с картой"}`, admin)
	var course struct {
		ID string `json:"id"`
	}
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &course) != nil || course.ID == "" {
		t.Fatalf("create shared course: %d %s", created.Code, created.Body.String())
	}
	var materialCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM subject_materials WHERE subject_id=$1`, course.ID).Scan(&materialCount); err != nil || materialCount != 0 {
		t.Fatalf("expected a new shared course without materials: count=%d err=%v", materialCount, err)
	}
	response := upload(f, "/admin/subjects/"+course.ID+"/graph/competency-map", "file", "map.xlsx", "application/octet-stream", mapBook(t), admin)
	if response.Code != http.StatusAccepted {
		t.Fatalf("map import for material-free course: %d %s", response.Code, response.Body.String())
	}
	var maps, builds int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM course_graph_maps WHERE subject_id=$1`, course.ID).Scan(&maps); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM course_graph_jobs WHERE subject_id=$1 AND source='competency_map'`, course.ID).Scan(&builds); err != nil {
		t.Fatal(err)
	}
	if maps != 1 || builds != 1 {
		t.Fatalf("map import should save its own input and queue map build: maps=%d builds=%d", maps, builds)
	}
}

// A real book weighs hundreds of KiB, far above the 32 KiB the API allows a
// JSON body; the map route lifts that limit up to the book's own.
func TestImportMapAcceptsABookLargerThanAJSONBody(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	admin := f.admin(t)
	book := mapBook(t)
	archive, err := zip.NewReader(bytes.NewReader(book), int64(len(book)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, part := range archive.File {
		if err := w.Copy(part); err != nil {
			t.Fatal(err)
		}
	}
	// A picture of the book: stored as is and never read by the importer.
	picture, err := w.CreateHeader(&zip.FileHeader{Name: "xl/media/image1.png", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(picture, rand.Reader, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	res := upload(f, "/admin/subjects/subject:test/graph/competency-map", "file", "map.xlsx", "application/octet-stream", buf.Bytes(), admin)
	if res.Code != http.StatusAccepted {
		t.Fatalf("1 MiB map: %d %s", res.Code, res.Body.String())
	}
}
