package app

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestExamCompetencyMapImportIsNotSupported(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	admin := f.admin(t)
	created := f.request("POST", "/admin/subjects", `{"name":"Общий курс"}`, admin)
	var subject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &subject); err != nil || subject.ID == "" {
		t.Fatalf("create course: %s (%v)", created.Body.String(), err)
	}
	response := f.request("POST", "/exam/admin/subjects/"+subject.ID+"/graph/competency-map", "", admin)
	if response.Code != http.StatusNotFound {
		t.Fatalf("exam map import: got %d, want 404: %s", response.Code, response.Body.String())
	}
}
