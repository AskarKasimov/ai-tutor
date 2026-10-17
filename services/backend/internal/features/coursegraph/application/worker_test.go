package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
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
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "ready"}}, items, nil, nil, nil)
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
	w := New(requestRepo{snapshot: coursegraph.Snapshot{Status: "processing"}}, nil, nil, nil, nil)
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
