package exam

import (
	"encoding/json"
	"reflect"
	"testing"
)

const agentRubric = "Аналитическая рубрика (макс. 2):\nПункт рубрики 1 «Тип агента»: 1 — выбран агент с внутренней моделью; 0 — простой рефлекторный.\nПункт рубрики 2 «Связь со средой»: 1 — названа частичная наблюдаемость и объяснено зачем нужна модель; 0 — нет связи."

func TestParseRubricPreservesIndividualScales(t *testing.T) {
	for _, tc := range []struct {
		text       string
		max, items int
	}{
		{agentRubric, 2, 2},
		{"Аналитическая рубрика (макс. 6):\nК1 «Суть»: 2 — верно; 1 — частично; 0 — нет.\nК2 «Связь»: 2 — верно; 1 — частично; 0 — нет.\nК3 «Пример»: 2 — верно; 1 — частично; 0 — нет.", 6, 3},
		{"Балл (макс. 1): 1 — все четыре; 0 — иначе.", 1, 1},
	} {
		items, err := ParseRubric(tc.text)
		if err != nil || len(items) != tc.items {
			t.Fatalf("parse=%+v err=%v", items, err)
		}
		sum := 0
		for _, i := range items {
			sum += i.Levels[len(i.Levels)-1]
		}
		if sum != tc.max {
			t.Fatal(sum)
		}
	}
	for _, s := range []string{"Правильный ответ своими словами достаточен.", "Аналитическая рубрика: полнота и смысл ответа на этот вопрос."} {
		items, err := ParseRubric(s)
		if err != nil || len(items) > 0 {
			t.Fatalf("legacy=%+v %v", items, err)
		}
	}
	for _, s := range []string{"Аналитическая рубрика (макс. 3):\nК1 «Суть»: 1 — верно; 0 — нет.", "Балл (макс. 2): оценить по смыслу"} {
		if _, err := ParseRubric(s); err == nil {
			t.Fatal("accepted ambiguous scale")
		}
	}
}

func rubricResult(t *testing.T, scores ...int) *RubricResult {
	t.Helper()
	spec, err := ParseRubric(agentRubric)
	if err != nil {
		t.Fatal(err)
	}
	r := &RubricResult{}
	for i, s := range scores {
		r.Items = append(r.Items, RubricMark{RubricItem: spec[i], Score: s, Evidence: []int{2}, Quote: "Содержание ответа", Reason: "Содержание ответа"})
	}
	return r
}

func TestRubricGradesAndEvidence(t *testing.T) {
	weighted := RubricResult{Items: []RubricMark{{RubricItem: RubricItem{Levels: []int{0, 1, 2}}, Score: 2}, {RubricItem: RubricItem{Levels: []int{0, 1, 2}}, Score: 1}}}
	if weighted.Grade(false, false) != GradeB {
		t.Fatal("75 percent must be B")
	}
	for _, tc := range []struct {
		s              []int
		help, complete bool
		want           Grade
	}{
		{[]int{1, 1}, false, true, GradeA}, {[]int{1, 1}, true, true, GradeB}, {[]int{1, 0}, false, false, GradeC}, {[]int{0, 0}, false, false, GradeD}, {[]int{1, 1}, false, false, GradeB},
	} {
		if got := rubricResult(t, tc.s...).Grade(tc.help, tc.complete); got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
	}
	r := rubricResult(t, 1, 0)
	spec, _ := ParseRubric(agentRubric)
	if err := r.Validate(spec, []Turn{{Line: 2, Speaker: SpeakerGeek}}); err == nil {
		t.Fatal("teacher words accepted")
	}
	if err := r.Validate(spec, []Turn{{Line: 2, Speaker: SpeakerStudent, Meta: true}}); err == nil {
		t.Fatal("meta message accepted")
	}
	if err := r.Validate(spec, []Turn{{Line: 2, Speaker: SpeakerStudent, Text: "Содержание ответа"}}); err != nil {
		t.Fatal(err)
	}
	r.Items[0].Quote = "Расчёт, которого в ответе нет"
	if err := r.Validate(spec, []Turn{{Line: 2, Speaker: SpeakerStudent, Text: "Содержание ответа"}}); err == nil {
		t.Fatal("invented quote accepted")
	}
	r.Items[0].Quote = "Содержание ответа"
	r.Items[0].Score = 2
	if err := r.Validate(spec, []Turn{{Line: 2, Speaker: SpeakerStudent}}); err == nil {
		t.Fatal("invented scoring level")
	}
}

func TestRubricFollowupAccumulatesAndSurvivesPersistence(t *testing.T) {
	n := Node{ID: "a", Title: "Агент", Questions: []string{"Какой агент и почему?"}, AssessmentGuidance: agentRubric}
	e, _ := NewExam(Graph{Nodes: []Node{n}}, 0, nil)
	no, yes := false, true
	next := e.Apply(Evaluation{Verdict: VerdictAnswered, AnswerComplete: &no, Rubric: rubricResult(t, 1, 0)})
	if next.Kind != KindFollowup || e.Nodes["a"].Assisted {
		t.Fatalf("partial result bypassed clarification: %+v", next)
	}
	saved, _ := json.Marshal(e)
	var restored Exam
	if err := json.Unmarshal(saved, &restored); err != nil {
		t.Fatal(err)
	}
	before := restored.Nodes["a"].Rubric
	restored.Apply(Evaluation{Verdict: VerdictPause})
	if !reflect.DeepEqual(before, restored.Nodes["a"].Rubric) {
		t.Fatal("pause altered scores")
	}
	restored.Apply(Evaluation{Verdict: VerdictAnswered, AnswerComplete: &yes, Rubric: rubricResult(t, 1, 1)})
	s := restored.Nodes["a"]
	if s.Grade == nil || *s.Grade != GradeA || s.Status != StatusIndependent {
		t.Fatalf("followup=%+v", s)
	}
	e, _ = NewExam(Graph{Nodes: []Node{n}}, 0, nil)
	e.Apply(Evaluation{Verdict: VerdictAnswered, AnswerComplete: &no, Rubric: rubricResult(t, 1, 0)})
	e.Apply(Evaluation{Verdict: VerdictDontKnow})
	if e.Nodes["a"].Grade == nil || *e.Nodes["a"].Grade != GradeC || !e.Nodes["a"].Rubric.HasScore() {
		t.Fatal("followup forgot demonstrated knowledge")
	}
	e, _ = NewExam(Graph{Nodes: []Node{n}}, 0, nil)
	e.Apply(Evaluation{Verdict: VerdictHintRequested})
	e.Apply(Evaluation{Verdict: VerdictAnswered, AnswerComplete: &yes, Rubric: rubricResult(t, 1, 1)})
	if e.Nodes["a"].Grade == nil || *e.Nodes["a"].Grade != GradeB || !e.Nodes["a"].Rubric.Complete() {
		t.Fatal("help must cap the letter without changing rubric points")
	}
}
