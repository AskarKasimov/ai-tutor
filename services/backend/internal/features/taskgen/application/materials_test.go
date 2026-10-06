package application

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitChunksPreservesMaterialAcrossBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, text string
	}{
		{"code", strings.Repeat("if x > 0:\r\n    print('значение 😀')\n\n", 100)},
		{"long token", strings.Repeat("я😀", 2000)},
		{"leading whitespace", strings.Repeat(" ", 2500) + "Вопрос"},
		{"internal whitespace", "Первая строка" + strings.Repeat(" ", 2500) + "Вторая строка"},
		{"trailing whitespace", "Ответ" + strings.Repeat(" ", 2500)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := splitChunks(tc.text, 1200)
			if got := strings.Join(chunks, ""); got != tc.text {
				t.Fatalf("chunking changed material: got %d bytes, want %d", len(got), len(tc.text))
			}
			for i, chunk := range chunks {
				if !utf8.ValidString(chunk) || strings.TrimSpace(chunk) == "" {
					t.Fatalf("chunk %d is invalid or has no content", i)
				}
				if (tc.name == "code" || tc.name == "long token") && utf8.RuneCountInString(chunk) > 1200 {
					t.Fatalf("chunk %d exceeds the target size", i)
				}
			}
		})
	}
}
