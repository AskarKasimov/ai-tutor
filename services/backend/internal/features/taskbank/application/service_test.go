package application

import (
	"context"
	"reflect"
	"testing"
)

type fakeRepository struct {
	filter       SearchFilter
	searchResult []TaskSummary
}

func (r *fakeRepository) Search(_ context.Context, filter SearchFilter) ([]TaskSummary, error) {
	r.filter = filter
	return r.searchResult, nil
}

func (r *fakeRepository) Profile(context.Context, string) (TaskProfile, error) {
	return TaskProfile{}, nil
}

func TestSearchPassesAllFiltersAndAppliesSafeLimit(t *testing.T) {
	include := false
	want := SearchFilter{
		OutcomeID: "outcome-1", CompetencyID: "competency-1", ConstituentID: "constituent-1",
		TaxonomyCode: "analysis", ALDLevelCode: "advanced", TopicLevelCode: "intermediate",
		Origin: "ai_generated", SectionCode: "Р.6", CurriculumCompetencyCode: "ОПК-8",
		Importance: 5, IncludeInTest: &include, Limit: 75,
	}
	repository := &fakeRepository{searchResult: []TaskSummary{{ID: "task-1", Options: []string{"A"}}}}
	service := New(repository)
	got, err := service.Search(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repository.filter, want) || len(got) != 1 || got[0].ID != "task-1" {
		t.Fatalf("search filter/result = %#v/%#v", repository.filter, got)
	}
	for _, limit := range []int32{0, -1, 101} {
		filter := SearchFilter{Limit: limit}
		if _, err := service.Search(context.Background(), filter); err != nil {
			t.Fatal(err)
		}
		if repository.filter.Limit != 50 {
			t.Errorf("limit %d normalized to %d, want 50", limit, repository.filter.Limit)
		}
	}
	filter := SearchFilter{Limit: 100}
	if _, err := service.Search(context.Background(), filter); err != nil || repository.filter.Limit != 100 {
		t.Fatalf("maximum supported limit: filter=%#v error=%v", repository.filter, err)
	}
}
