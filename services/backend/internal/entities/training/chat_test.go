package training

import (
	"encoding/json"
	"testing"
)

func TestCheckBecomesStaleAfterMessage(t *testing.T) {
	check := Check{Revision: 3}
	if check.IsCurrent(4) {
		t.Fatal("old check cannot authorize Next")
	}
	if !check.IsCurrent(3) {
		t.Fatal("same revision must remain usable")
	}
}

func TestLegacyAttemptWithoutAnswerSourceMeansVoice(t *testing.T) {
	var attempt Attempt
	if err := json.Unmarshal([]byte(`{"transcription_id":"tr-1"}`), &attempt); err != nil {
		t.Fatal(err)
	}
	if attempt.EffectiveAnswerSource() != AnswerSourceVoice {
		t.Fatalf("legacy attempt source = %q, want voice", attempt.EffectiveAnswerSource())
	}
	textAttempt := Attempt{AnswerSource: AnswerSourceText}
	if textAttempt.EffectiveAnswerSource() != AnswerSourceText || textAttempt.TranscriptionID != "" {
		t.Fatalf("text attempt must not gain a synthetic transcription: %+v", textAttempt)
	}
}
