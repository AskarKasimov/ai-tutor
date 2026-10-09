package app

import (
	"encoding/json"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"net/http"
	"testing"
)

func TestLearningStateUnknownSubjectIsNotFound(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "learning-state@example.edu")
	w := f.request(http.MethodGet, "/subjects/missing/learning-state", "", access)
	if w.Code != 404 {
		t.Fatalf("unknown subject %d %s", w.Code, w.Body.String())
	}
}
func TestLearningStateOrdersCompletedByTimeThenIDAndKeepsNewActive(t *testing.T) {
	f, access, _, owner, d := trainingFixture(t, 1)
	for _, id := range []string{"a-completed", "z-completed"} {
		copy := d
		copy.ID = id
		if _, _, err := f.app.diagnosticStore.Create(t.Context(), owner, id, id, copy); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE diagnostic_sessions SET completed_at='2026-10-09 12:00:00+00' WHERE status='completed'`); err != nil {
		t.Fatal(err)
	}
	active := d
	active.ID = "active-new"
	active.Status = diagnostic.StatusActive
	active.CurrentCompetency = 0
	active.Answers = nil
	active.AcceptedRequests = map[string]diagnostic.AcceptedRequest{}
	if _, _, err := f.app.diagnosticStore.Create(t.Context(), owner, "active-new", "active-new", active); err != nil {
		t.Fatal(err)
	}
	w := f.request("GET", "/subjects/"+d.Variant.SubjectID+"/learning-state", "", access)
	var state diagnostic.LearningState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || w.Code != 200 || state.DiagnosticSessionID != "z-completed" || state.ActiveSessionID != "active-new" || !state.TrainingAvailable {
		t.Fatalf("state %d %s %v", w.Code, w.Body.String(), err)
	}
	other, _, _ := f.register(t, "learning-state-other@example.edu")
	foreign := f.request("GET", "/subjects/"+d.Variant.SubjectID+"/learning-state", "", other)
	if err := json.Unmarshal(foreign.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.TrainingAvailable || state.DiagnosticStatus != "not_started" {
		t.Fatalf("foreign state %s", foreign.Body.String())
	}
}
