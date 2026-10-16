package graph

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

type stored struct{ g *exam.Graph }

func (s stored) Current(context.Context, string) (exam.Graph, bool, error) {
	if s.g == nil {
		return exam.Graph{}, false, nil
	}
	return *s.g, true, nil
}

func TestChainPrefersTheStoredGraphOverTheFixture(t *testing.T) {
	ctx := context.Background()
	fixture := Fixture{Path: "../../../../../../../api/fixtures/course-graph.json"}
	g, ok, err := Chain{stored{}, fixture}.Current(ctx, "s")
	if err != nil || !ok || len(g.Nodes) != 10 || g.Version == "" || g.Nodes[0].Outcomes == nil {
		t.Fatalf("fixture fallback: %v %v %d", ok, err, len(g.Nodes))
	}
	db := exam.Graph{Version: "db", Nodes: []exam.Node{{ID: "n", Title: "Тема"}}}
	if g, _, _ := (Chain{stored{&db}, fixture}).Current(ctx, "s"); g.Version != "db" {
		t.Fatalf("stored graph not preferred: %s", g.Version)
	}
	if _, ok, err := (Chain{stored{}, Fixture{}}).Current(ctx, "s"); ok || err != nil {
		t.Fatalf("no graph: %v %v", ok, err)
	}
}

func TestFixtureServesOnlyItsCourse(t *testing.T) {
	ctx := context.Background()
	fixture := Fixture{Path: "../../../../../../../api/fixtures/course-graph.json", SubjectID: "demo"}
	if _, ok, err := fixture.Current(ctx, "demo"); !ok || err != nil {
		t.Fatalf("demo course: %v %v", ok, err)
	}
	if _, ok, err := fixture.Current(ctx, "other"); ok || err != nil {
		t.Fatalf("other course got the fixture: %v %v", ok, err)
	}
}

func TestFixtureRejectsAnInvalidGraph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, []byte(`{"nodes":[{"id":"a","title":"A","prerequisites":["a"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (Fixture{Path: path}).Current(context.Background(), "s"); err == nil {
		t.Fatal("cyclic fixture accepted")
	}
}
