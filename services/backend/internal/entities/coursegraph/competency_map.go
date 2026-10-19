package coursegraph

import (
	"cmp"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A competency map is the second source of a course graph besides the
// materials (decision of 19.10.2026). Its template is the course map of
// competencies: Субкомпетенция → Составляющая → ОР with Bloom level,
// importance, «Что должно войти в тест», Задание and Критерии оценивания.
//
// The graph follows the variant of v0.0.2 (competency-map-v4): for every
// competency with at least three outcomes marked TRUE that have a task, the
// outcome with the highest Bloom level (then importance, then map order) is
// a top-level topic and the two simplest of the others (Bloom ascending,
// importance descending, map order) are its prerequisites.

// MapVersion is the version of a graph built from a competency map.
const MapVersion = "competency-map-v1"

// SourceMap marks a graph built from a competency map.
const SourceMap = "competency_map"

// mapColumns are found by the prefix of their header: the template adds
// English glosses in brackets.
var mapColumns = map[string]string{
	"competency":  "Субкомпетенция",
	"constituent": "Составляющая",
	"outcome":     "Образовательный результат",
	"bloom":       "Уровень по Блуму",
	"importance":  "Важность",
	"include":     "Что должно войти в тест",
	"task":        "Задание",
	"criteria":    "Критерии оценивания",
	"evidence":    "Свидетельство",
}

var requiredMapColumns = []string{"competency", "constituent", "outcome", "bloom", "importance", "include", "task", "criteria"}

// bloomRanks orders the Bloom levels of the template; the nouns of the
// previous ML map are accepted as well.
var bloomRanks = map[string]int{
	"помнить": 1, "знание": 1,
	"понимать": 2, "понимание": 2,
	"применять": 3, "применение": 3,
	"анализировать": 4, "анализ": 4,
	"оценивать": 5, "оценка": 5,
	"создавать": 6, "синтез": 6,
}

// MapSheet is one worksheet of a competency map book.
type MapSheet struct {
	Name string
	Rows [][]string
}

// MapIssue is a problem in a cell of the map; Row counts from 1 as in Excel.
type MapIssue struct {
	Row     int    `json:"row"`
	Column  string `json:"column"`
	Message string `json:"message"`
}

// SkippedCompetency is a competency left out of the graph and why.
type SkippedCompetency struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// MapGraph is the graph of a competency map in the shape of a course-graph
// build, so Geek and the admin read it as they read a build.
type MapGraph struct {
	Graph   json.RawMessage
	Sheet   string
	Topics  int
	Skipped []SkippedCompetency
}

// MapError rejects a map: Issues name the cells, Sheets lists the candidate
// sheets when the book has several and none was chosen.
type MapError struct {
	Code    string
	Message string
	Issues  []MapIssue
	Sheets  []string
}

func (e *MapError) Error() string { return e.Message }

// TemplateSheets are the sheets whose header row has the map columns.
func TemplateSheets(sheets []MapSheet) []string {
	out := []string{}
	for _, s := range sheets {
		if _, _, ok := mapHeader(s.Rows); ok {
			out = append(out, s.Name)
		}
	}
	return out
}

// mapHeader finds the header row (the first non-empty one) and its columns.
func mapHeader(rows [][]string) (int, map[string]int, bool) {
	for i, row := range rows {
		if blank(row) {
			continue
		}
		columns := map[string]int{}
		for key, prefix := range mapColumns {
			for j, title := range row {
				if strings.HasPrefix(strings.TrimSpace(title), prefix) {
					columns[key] = j
					break
				}
			}
		}
		for _, key := range requiredMapColumns {
			if _, ok := columns[key]; !ok {
				return 0, nil, false
			}
		}
		return i, columns, true
	}
	return 0, nil, false
}

type mapOutcome struct {
	row                   int
	competency, outcome   string
	constituent, evidence string
	task, criteria        string
	bloom, importance     int
	order                 int
}

// BuildMapGraph builds the course graph from the chosen sheet; sheet may be
// empty when the book has exactly one sheet of the template.
func BuildMapGraph(sheets []MapSheet, sheet string) (MapGraph, error) {
	candidates := TemplateSheets(sheets)
	if sheet == "" {
		switch len(candidates) {
		case 0:
			return MapGraph{}, &MapError{Code: "COMPETENCY_MAP_INVALID",
				Message: "Не найден лист с колонками карты: Субкомпетенция, Составляющая, Образовательный результат, Уровень по Блуму, Важность, Что должно войти в тест, Задание, Критерии оценивания."}
		case 1:
			sheet = candidates[0]
		default:
			return MapGraph{}, &MapError{Code: "COMPETENCY_MAP_SHEET_REQUIRED",
				Message: "В книге несколько листов с картой; выберите лист.", Sheets: candidates}
		}
	} else if !slices.Contains(candidates, sheet) {
		return MapGraph{}, &MapError{Code: "COMPETENCY_MAP_INVALID",
			Message: fmt.Sprintf("Лист «%s» не найден или без колонок карты.", sheet), Sheets: candidates}
	}
	var rows [][]string
	for _, s := range sheets {
		if s.Name == sheet {
			rows = s.Rows
		}
	}
	outcomes, competencies, issues := readMapOutcomes(rows)
	if len(issues) > 0 {
		return MapGraph{}, &MapError{Code: "COMPETENCY_MAP_INVALID",
			Message: fmt.Sprintf("Карта на листе «%s» содержит ошибки.", sheet), Issues: issues}
	}
	graph, topics, skipped, err := mapGraph(competencies, outcomes)
	if err != nil {
		return MapGraph{}, err
	}
	if topics == 0 {
		return MapGraph{}, &MapError{Code: "COMPETENCY_MAP_EMPTY",
			Message: "Ни одна субкомпетенция не набрала трёх ОР с TRUE и заданием; граф не из чего собрать.", Issues: skippedIssues(skipped)}
	}
	return MapGraph{Graph: graph, Sheet: sheet, Topics: topics, Skipped: skipped}, nil
}

func skippedIssues(skipped []SkippedCompetency) []MapIssue {
	out := make([]MapIssue, len(skipped))
	for i, s := range skipped {
		out[i] = MapIssue{Column: mapColumns["competency"], Message: s.Name + ": " + s.Reason}
	}
	return out
}

// readMapOutcomes reads the outcomes that may enter the graph and every
// competency of the map in its order; the competency and constituent are
// filled on the first row of their group only.
func readMapOutcomes(rows [][]string) ([]mapOutcome, []string, []MapIssue) {
	header, columns, _ := mapHeader(rows)
	cell := func(row []string, key string) string {
		i, ok := columns[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	var (
		out                     []mapOutcome
		competencies            []string
		issues                  []MapIssue
		competency, constituent string
	)
	issue := func(row int, key, message string) {
		issues = append(issues, MapIssue{Row: row, Column: mapColumns[key], Message: message})
	}
	for i := header + 1; i < len(rows); i++ {
		row, line := rows[i], i+1
		if blank(row) {
			continue
		}
		if v := cell(row, "competency"); v != "" {
			competency = v
		}
		if v := cell(row, "constituent"); v != "" {
			constituent = v
		}
		outcome := cell(row, "outcome")
		if outcome == "" {
			continue
		}
		if competency == "" {
			issue(line, "competency", "У ОР нет субкомпетенции выше.")
			continue
		}
		if !slices.Contains(competencies, competency) {
			competencies = append(competencies, competency)
		}
		include := strings.ToUpper(cell(row, "include"))
		switch include {
		case "TRUE", "FALSE":
		case "":
			include = "FALSE"
		default:
			issue(line, "include", "Ожидается TRUE или FALSE.")
			continue
		}
		task, criteria := cell(row, "task"), cell(row, "criteria")
		// Only outcomes for the test with a task can be asked.
		if include != "TRUE" || task == "" {
			continue
		}
		bloom, ok := bloomRanks[strings.ToLower(cell(row, "bloom"))]
		if !ok {
			issue(line, "bloom", "Неизвестный уровень по Блуму: ожидается Помнить, Понимать, Применять, Анализировать, Оценивать или Создавать.")
			continue
		}
		importance, ok := importanceValue(cell(row, "importance"))
		if !ok {
			issue(line, "importance", "Ожидается целое число от 1 до 5.")
			continue
		}
		if criteria == "" {
			issue(line, "criteria", "У задания нет критериев оценивания.")
			continue
		}
		out = append(out, mapOutcome{row: line, competency: competency, constituent: constituent, outcome: outcome,
			evidence: cell(row, "evidence"), task: task, criteria: criteria, bloom: bloom, importance: importance, order: len(out)})
	}
	return out, competencies, issues
}

func importanceValue(value string) (int, bool) {
	number, err := strconv.ParseFloat(strings.Replace(value, ",", ".", 1), 64)
	if err != nil || number != float64(int(number)) || number < 1 || number > 5 {
		return 0, false
	}
	return int(number), true
}

func blank(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

type mapTopic struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Text               string   `json:"text"`
	Question           string   `json:"question"`
	AssessmentGuidance string   `json:"assessment_guidance,omitempty"`
	Provides           []string `json:"provides"`
	Requires           []string `json:"requires"`
	ParentTitle        string   `json:"parent_title"`
	Role               string   `json:"role"`
	Row                int      `json:"row"`
}

type mapEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Reason string `json:"reason,omitempty"`
}

func mapGraph(order []string, outcomes []mapOutcome) (json.RawMessage, int, []SkippedCompetency, error) {
	byCompetency := map[string][]mapOutcome{}
	for _, o := range outcomes {
		byCompetency[o.competency] = append(byCompetency[o.competency], o)
	}
	topics := []mapTopic{}
	prerequisites, hierarchy := []mapEdge{}, []mapEdge{}
	skipped := []SkippedCompetency{}
	for _, name := range order {
		pool := byCompetency[name]
		if len(pool) < 3 {
			skipped = append(skipped, SkippedCompetency{Name: name,
				Reason: fmt.Sprintf("ОР с TRUE и заданием — %d, нужно не меньше трёх: основное и два базовых.", len(pool))})
			continue
		}
		main := slices.MinFunc(pool, func(a, b mapOutcome) int {
			return cmp.Or(cmp.Compare(b.bloom, a.bloom), cmp.Compare(b.importance, a.importance), cmp.Compare(a.order, b.order))
		})
		rest := slices.DeleteFunc(slices.Clone(pool), func(o mapOutcome) bool { return o.order == main.order })
		slices.SortStableFunc(rest, func(a, b mapOutcome) int {
			return cmp.Or(cmp.Compare(a.bloom, b.bloom), cmp.Compare(b.importance, a.importance), cmp.Compare(a.order, b.order))
		})
		mainID := topicID(main)
		// Study order: the basic tasks come before the main one.
		for _, basic := range rest[:2] {
			topics = append(topics, newMapTopic(basic, "basic", nil))
			prerequisites = append(prerequisites, mapEdge{Source: topicID(basic), Target: mainID, Reason: "Базовое задание субкомпетенции «" + name + "»."})
			hierarchy = append(hierarchy, mapEdge{Source: mainID, Target: topicID(basic)})
		}
		topics = append(topics, newMapTopic(main, "main", []string{topicID(rest[0]), topicID(rest[1])}))
	}
	data, err := json.Marshal(struct {
		Version       string     `json:"version"`
		Source        string     `json:"source"`
		Topics        []mapTopic `json:"topics"`
		Prerequisites []mapEdge  `json:"prerequisites"`
		Hierarchy     []mapEdge  `json:"hierarchy"`
		Quality       struct {
			Acyclic bool `json:"acyclic"`
			Topics  int  `json:"topics"`
		} `json:"quality"`
		Skipped []SkippedCompetency `json:"skipped"`
	}{Version: MapVersion, Source: SourceMap, Topics: topics, Prerequisites: prerequisites, Hierarchy: hierarchy,
		Quality: struct {
			Acyclic bool `json:"acyclic"`
			Topics  int  `json:"topics"`
		}{Acyclic: true, Topics: len(topics)}, Skipped: skipped})
	return data, len(topics), skipped, err
}

func topicID(o mapOutcome) string { return "map-row-" + strconv.Itoa(o.row) }

func newMapTopic(o mapOutcome, role string, requires []string) mapTopic {
	reference, guidance := splitCriteria(o.criteria)
	text := "Образовательный результат: " + o.outcome
	if o.evidence != "" {
		text += "\nСвидетельство: " + o.evidence
	}
	if reference != "" {
		text += "\n" + reference
	}
	if requires == nil {
		requires = []string{}
	}
	return mapTopic{ID: topicID(o), Title: o.outcome, Text: text, Question: o.task, AssessmentGuidance: guidance,
		Provides: []string{o.outcome}, Requires: requires, ParentTitle: o.competency, Role: role, Row: o.row}
}

// scoringLine starts the scoring part of the criteria: «Балл (макс. 1)» or an
// analytic rubric with its own К1, К2…, which are not Geek's К1–К4.
var scoringLine = regexp.MustCompile(`^(Балл|Аналитическая рубрика|К\d+\s)`)

var rubricCriterion = regexp.MustCompile(`^К(\d+)\s`)

// splitCriteria keeps the reference answer (Эталон, Допустимо) as the text of
// the topic and turns the scoring of the map into the teacher's rule for the
// grader, renaming the rubric's К1, К2… so the grader does not take them for
// its own criteria.
func splitCriteria(criteria string) (string, string) {
	var reference, scoring []string
	for _, line := range strings.Split(strings.ReplaceAll(criteria, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case len(scoring) > 0 || scoringLine.MatchString(line):
			scoring = append(scoring, rubricCriterion.ReplaceAllString(line, "Пункт рубрики $1 "))
		default:
			reference = append(reference, line)
		}
	}
	guidance := ""
	if len(scoring) > 0 {
		guidance = "Рубрика задания из карты компетенций (её пункты не совпадают с критериями К1–К4; учитывай их как ориентир, что считать полным ответом):\n" + strings.Join(scoring, "\n")
	}
	return strings.Join(reference, "\n"), guidance
}
