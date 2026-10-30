package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
)

type skipRepo struct {
	requestRepo
	saved json.RawMessage
}

func (r *skipRepo) SaveMap(_ context.Context, _, _, _ string, graph json.RawMessage, _ time.Time) (bool, error) {
	r.saved = graph
	return true, nil
}

func TestSkipMissingCriteriaRequiresAdminAndKeepsAuthoredQuestion(t *testing.T) {
	fallback, _ := json.Marshal(coursegraph.DefaultAssessmentGuidance)
	graph := json.RawMessage(`{"version":"competency-map-v1","topics":[{"id":"a","variants":[{"question":"skip","assessment_guidance":` + string(fallback) + `},{"question":"keep","assessment_guidance":"author"}]}],"prerequisites":[],"hierarchy":[]}`)
	repo := &skipRepo{requestRepo: requestRepo{snapshot: coursegraph.Snapshot{Status: "ready", GraphSource: coursegraph.SourceMap, Map: &coursegraph.MapFile{Filename: "map.xlsx", Sheet: "sheet"}}, mapGraph: graph}}
	w := New(repo, nil, nil, nil, nil, nil)
	if _, err := w.SkipMissingCriteria(context.Background(), user.User{Role: user.Student}, "s"); err == nil || repo.saved != nil {
		t.Fatal("non-admin was allowed to alter graph")
	}
	if _, err := w.SkipMissingCriteria(context.Background(), user.User{Role: user.Admin}, "s"); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Topics []struct {
			Question string
			Variants []struct{ Question string }
		}
		Skipped int `json:"assessment_skipped"`
	}
	if err := json.Unmarshal(repo.saved, &result); err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 1 || len(result.Topics) != 1 || result.Topics[0].Question != "keep" || len(result.Topics[0].Variants) != 1 {
		t.Fatalf("wrong graph: %s", repo.saved)
	}
}
