package courseapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
)

func TestBuildReportsTheProgressOfItsOwnBuildOnly(t *testing.T) {
	var polls atomic.Int32
	ownPolled := make(chan struct{}, 16)
	var seenID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graph/build/progress":
			// The first answer belongs to another build and must be skipped.
			id := "other#1"
			if polls.Add(1) > 1 {
				id = "subject#1"
				defer func() { ownPolled <- struct{}{} }()
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"build_id": id, "stage": "prerequisites",
				"stage_index": 3, "stage_count": 5, "done": 2, "total": 7})
		case "/graph/build":
			seenID = r.Header.Get("X-Build-ID")
			// Polls run one after another, so by the second poll of this
			// build the first one has been reported.
			for range 2 {
				<-ownPolled
			}
			_, _ = w.Write([]byte(`{"version":"course-graph-v1"}`))
		}
	}))
	defer server.Close()
	c := New(server.URL)
	c.poll = 5 * time.Millisecond
	var mu sync.Mutex
	var reports []coursegraph.BuildProgress
	returned := false
	_, err := c.Build(context.Background(), "subject#1", nil, func(p coursegraph.BuildProgress) {
		mu.Lock()
		defer mu.Unlock()
		if returned {
			t.Error("progress reported after Build returned")
		}
		reports = append(reports, p)
	})
	mu.Lock()
	returned = true
	mu.Unlock()
	if err != nil || seenID != "subject#1" {
		t.Fatalf("err = %v, X-Build-ID = %q", err, seenID)
	}
	want := coursegraph.BuildProgress{Stage: "prerequisites", StageIndex: 3, StageCount: 5, Done: 2, Total: 7}
	// Wait past a few poll intervals: nothing may arrive after Build returned.
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(reports) == 0 {
		t.Fatal("no progress reported")
	}
	for _, p := range reports {
		if p != want {
			t.Fatalf("report = %+v, want only the build's own progress", p)
		}
	}
}
