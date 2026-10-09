package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mapReadResponse struct {
	Revision     int64  `json:"revision"`
	ImportedAt   *int64 `json:"imported_at"`
	Competencies []struct {
		ID, Name     string
		Constituents []struct {
			ID, Name   string
			TopicLevel *string `json:"topic_level_code"`
			Sections   []struct {
				Code, Title  string
				Competencies []string `json:"curriculum_competencies"`
			} `json:"curriculum_sections"`
			Outcomes []struct {
				ID, Name   string
				Included   *bool   `json:"include_in_test"`
				Taxonomy   *string `json:"taxonomy_code"`
				ALD        *string `json:"ald_level_code"`
				Importance *int16  `json:"importance"`
				Content    *string `json:"educational_content"`
				Tasks      []struct {
					ID, Question, Origin string
					Options              []string
					VoiceInstruction     *string `json:"voice_instruction"`
				}
			}
		}
	}
}

func readMap(t *testing.T, f *fixture, cookie *http.Cookie) mapReadResponse {
	t.Helper()
	response := f.request("GET", "/subjects/subject:test/competency-map", "", cookie)
	var result mapReadResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != 200 {
		t.Fatalf("read map: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "reference_answer") || strings.Contains(response.Body.String(), "criteria") {
		t.Fatalf("map exposed grading material: %s", response.Body.String())
	}
	return result
}

func TestReadCompetencyMapIncludesEntireHierarchyAndTasklessOutcomes(t *testing.T) {
	f := newFixture(t)
	f.app.cfg.APIMode = "mock"
	admin := f.admin(t)
	student, _, _ := f.register(t, "map-reader@example.test")
	requireCode(t, f.request("GET", "/subjects/subject:test/competency-map", ""), 401, "UNAUTHORIZED")
	requireCode(t, f.request("GET", "/outcomes", "", student), 404, "NOT_FOUND")
	empty := readMap(t, f, student)
	if empty.Revision != 0 || empty.ImportedAt != nil || empty.Competencies == nil || len(empty.Competencies) != 0 {
		t.Fatalf("empty map: %#v", empty)
	}
	var csv strings.Builder
	csv.WriteString("Ком,Сост,ОР,Что должно войти в тест,Таксономия,Важность,Задание 1,Критерии 1,Задание 2,Критерии 2\nК1,С1,О0,TRUE,Знание,3,Вопрос,Критерий,Аналог,Критерий\n")
	for i := 1; i < 120; i++ {
		fmt.Fprintf(&csv, ",,О%d,FALSE,Анализ,5,,,,\n", i)
	}
	csv.WriteString("К2,С1,О0,,,,,,Другой вопрос,Другой критерий\n")
	response := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", []byte(csv.String()), admin)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	result := readMap(t, f, student)
	if result.Revision != 1 || result.ImportedAt == nil || *result.ImportedAt != f.now.Unix() || len(result.Competencies) != 2 {
		t.Fatalf("map metadata/parents: %#v", result)
	}
	first, second := result.Competencies[0], result.Competencies[1]
	if first.Name != "К1" || second.Name != "К2" || len(first.Constituents) != 1 || len(second.Constituents) != 1 || first.Constituents[0].ID == second.Constituents[0].ID {
		t.Fatalf("parent boundaries: %#v", result.Competencies)
	}
	constituent := first.Constituents[0]
	if constituent.TopicLevel != nil || constituent.Sections == nil || len(constituent.Sections) != 0 || len(constituent.Outcomes) != 120 {
		t.Fatalf("whole map truncated or partial profile invented: %#v", constituent)
	}
	var tasklessID string
	for _, outcome := range constituent.Outcomes {
		if outcome.ID == "" || outcome.Included == nil || outcome.Taxonomy == nil || outcome.Importance == nil || outcome.ALD != nil {
			t.Fatalf("outcome profile: %#v", outcome)
		}
		if outcome.Name == "О0" {
			if len(outcome.Tasks) != 2 || !*outcome.Included || *outcome.Taxonomy != "knowledge" || *outcome.Importance != 3 {
				t.Fatalf("authored outcome: %#v", outcome)
			}
			for _, task := range outcome.Tasks {
				if task.ID == "" || task.Origin != "authored" || task.Options == nil {
					t.Fatalf("authored task: %#v", task)
				}
			}
		} else {
			if *outcome.Included || *outcome.Taxonomy != "analysis" || *outcome.Importance != 5 || outcome.Tasks == nil || len(outcome.Tasks) != 0 {
				t.Fatalf("taskless outcome: %#v", outcome)
			}
			tasklessID = outcome.ID
		}
	}
	body, _ := json.Marshal(map[string]string{"subject_id": "subject:test", "outcome_id": tasklessID})
	request := httptest.NewRequest("POST", "/tasks/generate", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "map-taskless-outcome")
	request.AddCookie(student)
	generated := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(generated, request)
	if generated.Code != 201 {
		t.Fatalf("generate from map: %d %s", generated.Code, generated.Body.String())
	}
	result = readMap(t, f, student)
	found := false
	for _, outcome := range result.Competencies[0].Constituents[0].Outcomes {
		if outcome.ID == tasklessID {
			found = len(outcome.Tasks) == 1 && outcome.Tasks[0].Origin == "ai_generated" && outcome.Tasks[0].VoiceInstruction != nil
		}
	}
	if !found {
		t.Fatal("map omitted the generated task")
	}
}

func TestReadCompetencyMapPreservesMLPropertiesAndReplacesRevision(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	csv := "Компетенция;Составляющая;Образовательный результат;Уровень темы;Что должно войти в тест;Таксономия;Уровень ALDs;Важность;Раздел РПД · компетенции РПД;ОС;Задание1\nК;С;О;Продвинутый;FALSE;Анализ;Средний;4;\"Р.1 Введение\nОПК-2;ПК-2\";Учебный контекст;\"Экран: Вопрос. Варианты: 1 — A; 2 — B. Голосовая инструкция: Ответьте. Ответ: A\"\n"
	response := upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", []byte(csv), admin)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	result := readMap(t, f, admin)
	constituent := result.Competencies[0].Constituents[0]
	outcome := constituent.Outcomes[0]
	if constituent.TopicLevel == nil || *constituent.TopicLevel != "advanced" || len(constituent.Sections) != 1 || constituent.Sections[0].Code != "Р.1" || len(constituent.Sections[0].Competencies) != 2 || outcome.ALD == nil || *outcome.ALD != "intermediate" || outcome.Content == nil || *outcome.Content != "Учебный контекст" || len(outcome.Tasks) != 1 || len(outcome.Tasks[0].Options) != 2 {
		t.Fatalf("ML profile: %#v / %#v", constituent, outcome)
	}
	oldID := outcome.ID
	response = upload(f, "/admin/subjects/subject:test/competency-map/import", "file", "map.csv", "text/csv", []byte("Ком,Сост,ОР,Задание 1,Критерии 1\nНовая,С,О,Новый вопрос,К\n"), admin)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	result = readMap(t, f, admin)
	if result.Revision != 2 || len(result.Competencies) != 1 || result.Competencies[0].Name != "Новая" || result.Competencies[0].Constituents[0].Outcomes[0].ID == oldID {
		t.Fatalf("old map survived replacement: %#v", result)
	}
}
