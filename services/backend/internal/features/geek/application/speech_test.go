package application

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitSpeechKeepsPartsWithinTheLimitAtSentenceEnds(t *testing.T) {
	sentence := strings.Repeat("слово ", 15) + "конец. "
	text := strings.Repeat(sentence, 10)
	parts := SplitSpeech(text, 200)
	if len(parts) < 2 {
		t.Fatalf("parts = %d", len(parts))
	}
	for _, p := range parts {
		if utf8.RuneCountInString(p) > 200 || !strings.HasSuffix(p, ".") {
			t.Fatalf("part %q", p)
		}
	}
	if got := strings.Join(parts, " "); got != strings.TrimSpace(text) {
		t.Fatal("text changed")
	}
	if parts := SplitSpeech("Короткий вопрос?", 500); len(parts) != 1 {
		t.Fatalf("short text split: %v", parts)
	}
	if parts := SplitSpeech(strings.Repeat("я", 450), 200); len(parts) != 3 {
		t.Fatalf("long word parts = %d", len(parts))
	}
}
