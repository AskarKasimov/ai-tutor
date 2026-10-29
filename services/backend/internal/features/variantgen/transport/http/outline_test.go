package variantgenhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"go.uber.org/zap"
)

func TestOutlineAggregatesSelectionsForSameCompetency(t *testing.T) {
	comp := variant.CompetencyProfile{ID: "c", Name: "Competency"}
	makeTask := func(topic, outcome string) variant.VariantTask {
		return variant.VariantTask{Role: "main", Task: variant.TaskProfile{Constituent: variant.ConstituentProfile{ID: topic, Name: topic}, Outcome: variant.OutcomeProfile{ID: outcome, Name: outcome}}}
	}
	got := outlineDTO(variant.Variant{Competencies: []variant.CompetencySelection{
		{Competency: comp, Tasks: []variant.VariantTask{makeTask("a", "one")}},
		{Competency: comp, Tasks: []variant.VariantTask{makeTask("b", "two")}},
	}})
	if len(got.Competencies) != 1 || len(got.Competencies[0].Constituents) != 2 || got.Competencies[0].Constituents[1].Outcomes[0].ID != "two" {
		t.Fatalf("unexpected outline: %+v", got)
	}
}

type outlineRepository struct{ application.Repository }

func (outlineRepository) Preview(context.Context, string, application.BuildFunc) (variant.Variant, error) {
	return variant.Variant{Competencies: []variant.CompetencySelection{{Competency: variant.CompetencyProfile{ID: "comp", Name: "Длинная компетенция"}}}}, nil
}

type slowLabels struct{}

func (slowLabels) RequestBudget(int) time.Duration { return time.Second }
func (slowLabels) Labels(ctx context.Context, _ []string) (map[string]string, error) {
	select {
	case <-time.After(50 * time.Millisecond):
		return map[string]string{"Длинная компетенция": "Нейронные сети"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestOutlineExtendsHTTPWriteDeadlineForLabelGeneration(t *testing.T) {
	service := application.New(outlineRepository{}, nil, nil, nil).WithGraphLabels(slowLabels{})
	handler := New(service)
	server := httptest.NewUnstartedServer(httpx.RequestLogger(zap.NewNop(), http.HandlerFunc(handler.Preview)))
	server.Config.WriteTimeout = 5 * time.Millisecond
	server.Start()
	defer server.Close()
	client := server.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var outline DiagnosticOutline
	if err := json.NewDecoder(response.Body).Decode(&outline); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(outline.Competencies) != 1 || outline.Competencies[0].DisplayName != "Нейронные сети" {
		t.Fatalf("response = %+v", outline)
	}
}
