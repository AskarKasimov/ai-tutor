package gradingcatalog

import (
	"context"
	"strings"
	"testing"
)

func TestEmbeddedCatalogLoads(t *testing.T) {
	catalog, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gradingContext, err := catalog.ContextForTask(context.Background(), "ml_001")
	if err != nil {
		t.Fatal(err)
	}
	if gradingContext.TaskID != "ml_001" || gradingContext.Question == "" || gradingContext.ReferenceAnswer == "" {
		t.Fatalf("task context is incomplete: %#v", gradingContext)
	}
	if len(gradingContext.Criteria) == 0 || gradingContext.MaterialContext.Knowledge == "" || gradingContext.MaterialContext.Skills == "" {
		t.Fatal("grading context is incomplete")
	}
}

func TestTaskLookupTrimsID(t *testing.T) {
	catalog, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gradingContext, err := catalog.ContextForTask(context.Background(), "  ml_001  ")
	if err != nil {
		t.Fatal(err)
	}
	if gradingContext.TaskID != "ml_001" {
		t.Fatalf("task id: %q", gradingContext.TaskID)
	}
}

func TestMissingTaskReturnsError(t *testing.T) {
	catalog, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = catalog.ContextForTask(context.Background(), "missing"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDuplicateTaskIDRejected(t *testing.T) {
	data := `{
		"tasks": [
			{
				"task_id": "same",
				"question": "Q1",
				"voice_instruction": "Answer",
				"reference_answer": "A1",
				"outcome": {"title": "O", "taxonomy": "T", "level": "L"},
				"criteria": [{"key":"c1","description":"D"}],
				"material_context": {"knowledge": "K", "skills": "S"}
			},
			{
				"task_id": "same",
				"question": "Q2",
				"voice_instruction": "Answer",
				"reference_answer": "A2",
				"outcome": {"title": "O", "taxonomy": "T", "level": "L"},
				"criteria": [{"key":"c1","description":"D"}],
				"material_context": {"knowledge": "K", "skills": "S"}
			}
		]
	}`
	_, err := NewFromBytes([]byte(data))
	if err == nil || !strings.Contains(err.Error(), "duplicate task_id") {
		t.Fatalf("expected duplicate task_id error, got %v", err)
	}
}
