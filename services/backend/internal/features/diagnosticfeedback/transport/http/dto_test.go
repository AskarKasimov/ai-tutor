package http

import (
	"encoding/json"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
)

func TestFeedbackResponseSerializesEmptyStrengthsAsArray(t *testing.T) {
	encoded, err := json.Marshal(feedbackResponse(diagnostic.OverallFeedback{
		SessionID: "session-1",
		Strengths: []string{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["strengths"].([]any); !ok {
		t.Fatalf("strengths must be an array, got %s", encoded)
	}
}
