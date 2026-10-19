package coursegraph

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

var mapHeaderRow = []string{"№", "Субкомпетенция (Proficiency)", "Уровень темы (целевой)", "Составляющая (Observe)",
	"Образовательный результат (Observe Behaviour)", "Свидетельство (продукт работы)", "Уровень по Блуму", "Важность (1–5)",
	"Что должно войти в тест", "Задание (устная инструкция + визуальная опора)", "Критерии оценивания"}

func mapRow(competency, outcome, bloom, importance, include, task string) []string {
	return []string{"", competency, "", "1.1 Составляющая", outcome, "Устный ответ", bloom, importance, include, task,
		"Эталон: " + outcome + ".\nБалл (макс. 1): 1 — верно; 0 — иначе."}
}

type mapShape struct {
	Version string `json:"version"`
	Source  string `json:"source"`
	Topics  []struct {
		ID, Title, Text, Question, Role string
		ParentTitle                     string   `json:"parent_title"`
		Guidance                        string   `json:"assessment_guidance"`
		Requires                        []string `json:"requires"`
	} `json:"topics"`
	Prerequisites []struct{ Source, Target string } `json:"prerequisites"`
	Hierarchy     []struct{ Source, Target string } `json:"hierarchy"`
}

func buildShape(t *testing.T, rows [][]string) (MapGraph, mapShape) {
	t.Helper()
	g, err := BuildMapGraph([]MapSheet{{Name: "Карта", Rows: rows}}, "")
	if err != nil {
		t.Fatal(err)
	}
	var shape mapShape
	if err := json.Unmarshal(g.Graph, &shape); err != nil {
		t.Fatal(err)
	}
	return g, shape
}

func TestMapGraphFollowsTheVariantOfV002(t *testing.T) {
	rows := [][]string{mapHeaderRow,
		mapRow("К1. Агент", "Называет части агента", "Помнить", "4", "TRUE", "Назовите части агента."),
		mapRow("", "Объясняет рациональность", "Понимать", "3", "TRUE", "Объясните рациональность."),
		mapRow("", "Составляет PEAS", "Применять", "5", "TRUE", "Составьте PEAS."),
		mapRow("", "Находит ошибку в PEAS", "Анализировать", "4", "TRUE", "Найдите ошибку."),
		mapRow("", "Проектирует меру", "Анализировать", "5", "TRUE", "Предложите меру."),
		mapRow("", "Не входит в тест", "Создавать", "5", "FALSE", "Создайте агента."),
		mapRow("", "Без задания", "Создавать", "5", "TRUE", ""),
	}
	g, shape := buildShape(t, rows)
	if g.Sheet != "Карта" || g.Topics != 3 || shape.Version != MapVersion || shape.Source != SourceMap {
		t.Fatalf("graph = %+v %+v", g, shape)
	}
	titles := []string{}
	for _, topic := range shape.Topics {
		titles = append(titles, topic.Role+":"+topic.Title)
	}
	// Main: the highest Bloom level, then the higher importance; FALSE and
	// outcomes without a task never count. Basic: the simplest of the rest,
	// Bloom ascending, importance descending.
	want := []string{"basic:Называет части агента", "basic:Объясняет рациональность", "main:Проектирует меру"}
	if !slices.Equal(titles, want) {
		t.Fatalf("topics = %v, want %v", titles, want)
	}
	main := shape.Topics[2]
	if main.Question != "Предложите меру." || main.ParentTitle != "К1. Агент" || !slices.Equal(main.Requires, []string{shape.Topics[0].ID, shape.Topics[1].ID}) {
		t.Fatalf("main = %+v", main)
	}
	if len(shape.Prerequisites) != 2 || shape.Prerequisites[0].Target != main.ID || len(shape.Hierarchy) != 2 || shape.Hierarchy[0].Source != main.ID {
		t.Fatalf("edges = %+v %+v", shape.Prerequisites, shape.Hierarchy)
	}
	if !strings.Contains(main.Text, "Эталон: Проектирует меру.") || strings.Contains(main.Text, "Балл") || !strings.Contains(main.Guidance, "Балл (макс. 1)") {
		t.Fatalf("text %q guidance %q", main.Text, main.Guidance)
	}
}

func TestMapGraphSkipsACompetencyWithoutThreeOutcomes(t *testing.T) {
	rows := [][]string{mapHeaderRow,
		mapRow("К1. Агент", "А", "Помнить", "4", "TRUE", "Задание А"),
		mapRow("", "Б", "Понимать", "3", "TRUE", "Задание Б"),
		mapRow("", "В", "Применять", "5", "TRUE", "Задание В"),
		mapRow("К2. Логика", "Г", "Помнить", "4", "TRUE", "Задание Г"),
		mapRow("", "Д", "Понимать", "3", "FALSE", "Задание Д"),
	}
	g, _ := buildShape(t, rows)
	if g.Topics != 3 || len(g.Skipped) != 1 || g.Skipped[0].Name != "К2. Логика" || !strings.Contains(g.Skipped[0].Reason, "— 1,") {
		t.Fatalf("graph = %+v", g)
	}
	_, err := BuildMapGraph([]MapSheet{{Name: "Карта", Rows: rows[3:]}}, "")
	var mapErr *MapError
	if !errors.As(err, &mapErr) {
		t.Fatalf("map without a header: %v", err)
	}
	only := append([][]string{mapHeaderRow}, rows[4:]...)
	if _, err = BuildMapGraph([]MapSheet{{Name: "Карта", Rows: only}}, ""); !errors.As(err, &mapErr) || mapErr.Code != "COMPETENCY_MAP_EMPTY" {
		t.Fatalf("map with no complete competency: %v", err)
	}
}

func TestMapGraphRenamesTheRubricOfTheMap(t *testing.T) {
	reference, guidance := splitCriteria("Эталон: конфликт законов.\nДопустимо: «вред».\nАналитическая рубрика (макс. 2):\nК1 «Конфликт»: 1 — названы законы.\nК2 «Решение»: 1 — обосновано.")
	if reference != "Эталон: конфликт законов.\nДопустимо: «вред»." {
		t.Fatalf("reference = %q", reference)
	}
	if strings.Contains(guidance, "К1 «") || !strings.Contains(guidance, "Пункт рубрики 1 «Конфликт»") || !strings.Contains(guidance, "Пункт рубрики 2 «Решение»") {
		t.Fatalf("guidance = %q", guidance)
	}
}

func TestMapGraphNamesTheSheetAndTheBadCells(t *testing.T) {
	good := [][]string{mapHeaderRow,
		mapRow("К1. Агент", "А", "Помнить", "4", "TRUE", "Задание А"),
		mapRow("", "Б", "Понимать", "3", "TRUE", "Задание Б"),
		mapRow("", "В", "Применять", "5", "TRUE", "Задание В"),
	}
	bad := [][]string{mapHeaderRow,
		mapRow("К1. Агент", "А", "Вспоминать", "4", "TRUE", "Задание А"),
		mapRow("", "Б", "Понимать", "7", "TRUE", "Задание Б"),
		mapRow("", "В", "Применять", "5", "да", "Задание В"),
	}
	book := []MapSheet{{Name: "Карта", Rows: bad}, {Name: "Карта_ВСЯ", Rows: good}, {Name: "Легенда", Rows: [][]string{{"Поле", "Пояснение"}}}}
	if got := TemplateSheets(book); !slices.Equal(got, []string{"Карта", "Карта_ВСЯ"}) {
		t.Fatalf("template sheets = %v", got)
	}
	var mapErr *MapError
	if _, err := BuildMapGraph(book, ""); !errors.As(err, &mapErr) || mapErr.Code != "COMPETENCY_MAP_SHEET_REQUIRED" || len(mapErr.Sheets) != 2 {
		t.Fatalf("several sheets: %v", err)
	}
	if g, err := BuildMapGraph(book, "Карта_ВСЯ"); err != nil || g.Sheet != "Карта_ВСЯ" {
		t.Fatalf("chosen sheet: %+v %v", g, err)
	}
	if _, err := BuildMapGraph(book, "Легенда"); !errors.As(err, &mapErr) || mapErr.Code != "COMPETENCY_MAP_INVALID" {
		t.Fatalf("sheet without the template: %v", err)
	}
	_, err := BuildMapGraph(book, "Карта")
	if !errors.As(err, &mapErr) || len(mapErr.Issues) != 3 {
		t.Fatalf("bad cells: %v", err)
	}
	for i, want := range []MapIssue{{Row: 2, Column: "Уровень по Блуму"}, {Row: 3, Column: "Важность"}, {Row: 4, Column: "Что должно войти в тест"}} {
		if got := mapErr.Issues[i]; got.Row != want.Row || got.Column != want.Column {
			t.Fatalf("issue %d = %+v, want %+v", i, got, want)
		}
	}
}
