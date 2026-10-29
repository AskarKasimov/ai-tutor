package coursegraph

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestExplicitMapKeepsAllAuthoredQuestionsAndDependencies(t *testing.T) {
	data, err := os.ReadFile("../../../../../api/fixtures/intro-to-ai.weeks-1-2.ru.graph.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Nodes []struct {
			ID, Title, Content string
			Questions          []string
			Prerequisites      []string
		}
	}
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	header := append(append([]string{}, mapHeaderRow...), "ID темы", "Пререквизиты", "Текст темы")
	rows := [][]string{header}
	for _, n := range source.Nodes {
		row := mapRow("Авторские вопросы", n.Title, "Понимать", "3", "TRUE", n.Questions[0])
		rows = append(rows, append(row, n.ID, strings.Join(n.Prerequisites, "; "), n.Content))
	}
	built, shape := buildShape(t, rows)
	if built.Topics != len(source.Nodes) || len(built.Skipped) != 0 {
		t.Fatalf("lost authored topics: %+v", built)
	}
	for i, n := range source.Nodes {
		actual := shape.Topics[i]
		if actual.ID != n.ID || actual.Question != n.Questions[0] || !strings.HasPrefix(actual.Text, n.Content) || strings.Join(actual.Requires, ";") != strings.Join(n.Prerequisites, ";") {
			t.Fatalf("changed question, theory or dependencies of %s: %+v", n.ID, actual)
		}
	}
}

func TestExplicitMapRejectsBrokenGraphWithoutFallingBackToLegacySelection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ids, requires []string
	}{
		{"duplicate", []string{"a", "a"}, []string{"", ""}},
		{"unknown", []string{"a", "b"}, []string{"missing", ""}},
		{"cycle", []string{"a", "b"}, []string{"b", "a"}},
		{"empty-id", []string{"a", ""}, []string{"", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := append(append([]string{}, mapHeaderRow...), "ID темы", "Пререквизиты")
			rows := [][]string{header}
			for i, id := range tc.ids {
				rows = append(rows, append(mapRow("Группа", "Тема", "Помнить", "3", "TRUE", "Вопрос?"), id, tc.requires[i]))
			}
			if _, err := BuildMapGraph([]MapSheet{{Name: "Карта", Rows: rows}}); err == nil {
				t.Fatal("broken explicit graph accepted")
			}
		})
	}
	header := append(append([]string{}, mapHeaderRow...), "ID темы")
	if _, err := BuildMapGraph([]MapSheet{{Name: "Карта", Rows: [][]string{header, append(mapRow("Группа", "Тема", "Помнить", "3", "TRUE", "Вопрос?"), "a")}}}); err == nil {
		t.Fatal("partial graph headers accepted")
	}
}
