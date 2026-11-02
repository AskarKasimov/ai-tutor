package graph

import (
	"os"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

func TestDecodeRejectsAnInvalidGraph(t *testing.T) {
	if _, _, err := Decode([]byte(`{"nodes":[{"id":"a","title":"A","prerequisites":["a"]}]}`)); err == nil {
		t.Fatal("cyclic graph accepted")
	}
}

func TestDecodeReadsAGraphOfACompetencyMap(t *testing.T) {
	data, err := os.ReadFile("../../../../../../../api/fixtures/course-graph.json")
	if err != nil {
		t.Fatal(err)
	}
	if g, ok, err := Decode(data); err != nil || !ok || len(g.Nodes) != 10 || g.Nodes[0].Outcomes == nil {
		t.Fatalf("nodes: ok=%v err=%v", ok, err)
	}
	g, ok, err := Decode([]byte(`{"version":"competency-map-v1","source":"competency_map","topics":[` +
		`{"id":"b","title":"Базовое","text":"Эталон","question":"Задание Б","assessment_guidance":"Балл (макс. 1)"},` +
		`{"id":"m","title":"Основное","text":"Эталон","question":"Задание М","variants":[` +
		`{"question":"Задание М","content":"Эталон"},{"question":"Задание М2","rephrase":"Проще","content":"Эталон 2","assessment_guidance":"Рубрика"}]}],` +
		`"prerequisites":[{"source":"b","target":"m"}],"hierarchy":[{"source":"m","target":"b"}]}`))
	if err != nil || !ok || g.Source != "competency_map" {
		t.Fatalf("map graph: ok=%v err=%v source=%q", ok, err, g.Source)
	}
	if g.Nodes[0].AssessmentGuidance != "Балл (макс. 1)" || g.Nodes[1].MainQuestion() != "Задание М" || g.Nodes[1].Prerequisites[0] != "b" ||
		len(g.Nodes[1].Variants) != 2 || g.Nodes[1].Variants[1] != (exam.Variant{Question: "Задание М2", Rephrase: "Проще", Content: "Эталон 2", AssessmentGuidance: "Рубрика"}) {
		t.Fatalf("nodes = %+v", g.Nodes)
	}
}

func TestDecodeGivesTopicsOfAMapTheirCompetency(t *testing.T) {
	const topics = `"topics":[{"id":"a","title":"А","text":"т","question":"в","parent_title":" Компетенция "}],"prerequisites":[]`
	g, ok, err := Decode([]byte(`{"version":"competency-map-v1","source":"competency_map",` + topics + `}`))
	if err != nil || !ok || g.Nodes[0].Competency != "Компетенция" {
		t.Fatalf("map graph = %+v %v %v", g, ok, err)
	}
	// The parent of a topic built from materials is a section, not a competency.
	g, ok, err = Decode([]byte(`{"version":"v1",` + topics + `}`))
	if err != nil || !ok || g.Nodes[0].Competency != "" {
		t.Fatalf("materials graph = %+v %v %v", g, ok, err)
	}
}
