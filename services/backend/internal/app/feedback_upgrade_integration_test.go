package app

import (
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	feedbackpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/infrastructure/postgres"
)

func TestFeedbackUpgradePersistsAndKeepsFirstTextOfNewVersion(t *testing.T) {
	t.Parallel()
	f, access, _, owner, d := trainingFixture(t, 0)
	store := feedbackpg.NewStore(f.pool)
	if err := store.Save(t.Context(), owner, diagnostic.OverallFeedback{SessionID: d.ID, AnalysisVersion: 2, Summary: "Выберите правильный ответ."}); err != nil {
		t.Fatal(err)
	}
	w := f.request("GET", "/diagnostic-sessions/"+d.ID+"/feedback", "", access)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	upgraded, ok, err := store.Get(t.Context(), owner, d.ID)
	if err != nil || !ok || upgraded.AnalysisVersion != 3 || upgraded.Summary == "Выберите правильный ответ." {
		t.Fatalf("upgrade not persisted: %+v %v", upgraded, err)
	}
	other := upgraded
	other.Summary = "concurrent replacement"
	if err := store.Save(t.Context(), owner, other); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.Get(t.Context(), owner, d.ID)
	if err != nil || got.Summary != upgraded.Summary {
		t.Fatalf("same version overwritten: %+v %v", got, err)
	}
}
