package application

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type fakeRepository struct {
	filter                    SearchFilter
	searchResult              []TaskSummary
	profileResult             TaskProfile
	searchErr, profileErr     error
	searchCalls, profileCalls int
	taskID                    string
}

func (r *fakeRepository) Search(_ context.Context, filter SearchFilter) ([]TaskSummary, error) {
	r.searchCalls++
	r.filter = filter
	return r.searchResult, r.searchErr
}

func (r *fakeRepository) Profile(_ context.Context, taskID string) (TaskProfile, error) {
	r.profileCalls++
	r.taskID = taskID
	return r.profileResult, r.profileErr
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

func TestProfileDelegatesTaskIDAndReturnsProfile(t *testing.T) {
	want := TaskProfile{
		ID: "task-7", OutcomeID: "outcome-2", Origin: "authored",
		CurriculumSections: []CurriculumSection{{Code: "Р.2", CurriculumCompetencies: []string{"ОПК-8", "ПК-2"}}},
	}
	repository := &fakeRepository{profileResult: want}
	got, err := New(repository).Profile(context.Background(), "task-7")
	if err != nil {
		t.Fatal(err)
	}
	if repository.profileCalls != 1 || repository.taskID != "task-7" || !reflect.DeepEqual(got, want) {
		t.Fatalf("profile call/result = %d/%q/%#v", repository.profileCalls, repository.taskID, got)
	}
}

func TestStudentFacingTaskDTOsDoNotExposeEvaluationMaterial(t *testing.T) {
	voice := "Объясните"
	summary := TaskSummary{ID: "task-1", Question: "Вопрос", VoiceInstruction: &voice, Options: []string{}}
	profile := TaskProfile{ID: "task-1", Question: "Вопрос", VoiceInstruction: &voice, CurriculumSections: []CurriculumSection{}}
	for name, value := range map[string]any{"summary": summary, "profile": profile} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		body := string(encoded)
		if containsAny(body, "reference_answer", "criteria") {
			t.Errorf("%s DTO leaked evaluative material: %s", name, body)
		}
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
