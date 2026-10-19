package graph

import (
	"os"
	"testing"
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
		`{"id":"m","title":"Основное","text":"Эталон","question":"Задание М"}],` +
		`"prerequisites":[{"source":"b","target":"m"}],"hierarchy":[{"source":"m","target":"b"}]}`))
	if err != nil || !ok || g.Source != "competency_map" {
		t.Fatalf("map graph: ok=%v err=%v source=%q", ok, err, g.Source)
	}
	if g.Nodes[0].AssessmentGuidance != "Балл (макс. 1)" || g.Nodes[1].MainQuestion() != "Задание М" || g.Nodes[1].Prerequisites[0] != "b" {
		t.Fatalf("nodes = %+v", g.Nodes)
	}
}
