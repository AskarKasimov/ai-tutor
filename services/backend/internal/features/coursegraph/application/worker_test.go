package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
)

type requestRepo struct {
	snapshot coursegraph.Snapshot
	queued   bool
}

func (r requestRepo) Claim(context.Context, string, time.Time) (coursegraph.Job, error) {
	return coursegraph.Job{}, nil
}
func (r requestRepo) Finish(context.Context, coursegraph.Job, json.RawMessage, time.Time) (bool, error) {
	return false, nil
}
func (r requestRepo) Fail(context.Context, coursegraph.Job, string, time.Time) error { return nil }
func (r requestRepo) Read(context.Context, string) (coursegraph.Snapshot, error) {
	return r.snapshot, nil
}
func (r requestRepo) Request(context.Context, string, time.Time) (bool, error) { return r.queued, nil }

type requestMaterials []material.Material

func (m requestMaterials) List(context.Context, string) ([]material.Material, error) { return m, nil }

func TestRequestReportsClassifiedMaterialIDsWithoutPollingAsProcessing(t *testing.T) {
	week := int32(1)
	items := requestMaterials{{ID: "needs-classification", Status: material.StatusParsed,
		Week: &week, Cleaning: &material.Cleaning{Status: "ready"}}}
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "ready"}}, items, nil, nil, nil, nil)
	_, err := w.Request(context.Background(), user.User{Role: user.Admin}, "subject")
	if err == nil {
		t.Fatal("expected incomplete metadata error")
	}
	failure, ok := err.(interface{ Error() string })
	if !ok || failure.Error() != "MATERIAL_METADATA_INCOMPLETE" {
		t.Fatalf("error = %v", err)
	}
}

func TestRequestKeepsConflictPrecedenceDuringActiveBuild(t *testing.T) {
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "processing"}}, nil, nil, nil, nil, nil)
	_, err := w.Request(context.Background(), user.User{Role: user.Admin}, "subject")
	if err == nil || err.Error() != "GRAPH_BUILD_IN_PROGRESS" {
		t.Fatalf("error = %v", err)
	}
}

func TestTopicQuestionsAreThePreparedQuestionsOfTheGraph(t *testing.T) {
	graph := json.RawMessage(`{"topics":[{"question":" Почему ACO находит короткий путь? "},{"question":""},{"title":"без вопроса"}]}`)
	if got := TopicQuestions(graph); len(got) != 1 || got[0] != "Почему ACO находит короткий путь?" {
		t.Fatalf("questions = %q", got)
	}
	if got := TopicQuestions(json.RawMessage(`not json`)); len(got) != 0 {
		t.Fatalf("questions of an invalid graph = %q", got)
	}
}

type snapshotRepo struct {
	Repository
	snapshot coursegraph.Snapshot
}

func (r snapshotRepo) Read(context.Context, string) (coursegraph.Snapshot, error) {
	return r.snapshot, nil
}

type defaultGraph struct {
	graph exam.Graph
	ok    bool
	err   error
}

func (d defaultGraph) Current(context.Context, string) (exam.Graph, bool, error) {
	return d.graph, d.ok, d.err
}

var admin = user.User{ID: "admin", Role: user.Admin}

func TestReadShowsTheFixtureUntilTheFirstBuild(t *testing.T) {
	fixture := defaultGraph{ok: true, graph: exam.Graph{Version: "fixture:1", Nodes: []exam.Node{
		{ID: "a", Title: "Агент", Content: "Теория", Outcomes: []string{"различать агентов"}, Questions: []string{"Что такое агент?"}},
		{ID: "b", Title: "Среда", Prerequisites: []string{"a"}},
	}}}
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "not_requested", Graph: json.RawMessage("null")}}, nil, nil, nil, nil, fixture)
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceFixture {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
	var shape struct {
		Topics []struct {
			ID, Title, Question, Text string
			Provides                  []string
		}
		Prerequisites []struct{ Source, Target string }
		Hierarchy     []struct{ Source, Target string }
	}
	if err := json.Unmarshal(got.Graph, &shape); err != nil {
		t.Fatal(err)
	}
	if len(shape.Topics) != 2 || shape.Topics[0].Question != "Что такое агент?" || shape.Topics[0].Text != "Теория" ||
		len(shape.Topics[0].Provides) != 1 || shape.Hierarchy == nil {
		t.Fatalf("topics = %+v", shape)
	}
	if len(shape.Prerequisites) != 1 || shape.Prerequisites[0].Source != "a" || shape.Prerequisites[0].Target != "b" {
		t.Fatalf("prerequisites = %+v", shape.Prerequisites)
	}
}

func TestReadShowsTheBuildOnceItIsPublished(t *testing.T) {
	built := json.RawMessage(`{"topics":[{"id":"x","title":"Собранная тема"}]}`)
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "ready", Graph: built}}, nil, nil, nil, nil,
		defaultGraph{err: errors.New("the fixture must not be read")})
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceBuilt || string(got.Graph) != string(built) {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
}

func TestReadWithoutAFixtureHasNoGraph(t *testing.T) {
	w := New(snapshotRepo{snapshot: coursegraph.Snapshot{Status: "not_requested", Graph: json.RawMessage("null")}}, nil, nil, nil, nil, defaultGraph{})
	got, err := w.Read(context.Background(), admin, "subject")
	if err != nil || got.GraphSource != coursegraph.SourceNone || string(got.Graph) != "null" {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
}
