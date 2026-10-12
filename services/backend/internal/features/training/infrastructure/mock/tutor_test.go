package mock

import (
	"context"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

func TestTutorOffersStepwiseHelpInsteadOfAlwaysConfirming(t *testing.T) {
	tutor := Tutor{}
	cases := []struct {
		name     string
		input    training.TutorContext
		pedagogy training.Pedagogy
	}{
		{"no attempt", training.TutorContext{Kind: training.ChatKindMessage}, training.PedagogyQuestion},
		{"stuck", training.TutorContext{Kind: training.ChatKindMessage, StudentText: "не знаю"}, training.PedagogyHint},
		{"correct check", training.TutorContext{Kind: training.ChatKindCheck, Check: &training.Check{Score: 2, MaxScore: 2, Verdict: "correct"}}, training.PedagogyConfirmation},
		{"incorrect check", training.TutorContext{Kind: training.ChatKindCheck, Check: &training.Check{Score: 0, MaxScore: 2, Verdict: "incorrect"}}, training.PedagogyHint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tutor.Reply(context.Background(), tc.input)
			if err != nil || got.Pedagogy != tc.pedagogy || got.AssistantText == "" {
				t.Fatalf("unexpected mock reply: %+v %v", got, err)
			}
		})
	}
}
