package translator_test

import (
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/translator"
)

func TestSplitAndJoinChunks(t *testing.T) {
	sampleLecture := `# Lecture 1: Search

## Слайд 1

Introduction to search problems.
A search problem defines states and actions.

## Слайд 2

Breadth-first search algorithm expands shallowest node first.
Queue data structure is FIFO.

## Слайд 3

Depth-first search algorithm expands deepest node first.
Stack data structure is LIFO.

## Слайд 4

Heuristic search: A* search algorithm uses $f(n) = g(n) + h(n)$.
`

	t.Run("split by slides with large maxChars", func(t *testing.T) {
		chunks := translator.SplitIntoChunks(sampleLecture, 10000)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk for large maxChars, got %d", len(chunks))
		}
		if chunks[0].Index != 0 {
			t.Errorf("expected chunk index 0, got %d", chunks[0].Index)
		}
	})

	t.Run("split into multiple slide chunks", func(t *testing.T) {
		chunks := translator.SplitIntoChunks(sampleLecture, 150)
		if len(chunks) < 3 {
			t.Fatalf("expected at least 3 chunks with limit 150, got %d", len(chunks))
		}

		joined := translator.JoinChunks(chunks)
		for _, expectedSubstring := range []string{
			"# Lecture 1: Search",
			"## Слайд 1",
			"## Слайд 2",
			"## Слайд 3",
			"## Слайд 4",
			"$f(n) = g(n) + h(n)$",
		} {
			if !strings.Contains(joined, expectedSubstring) {
				t.Errorf("joined text missing %q", expectedSubstring)
			}
		}
	})

	t.Run("empty input", func(t *testing.T) {
		chunks := translator.SplitIntoChunks("", 500)
		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
		}
		if joined := translator.JoinChunks(chunks); joined != "" {
			t.Errorf("expected empty joined string, got %q", joined)
		}
	})

	t.Run("paragraphs without slide headers", func(t *testing.T) {
		text := "Paragraph one with some text.\n\nParagraph two with more details.\n\nParagraph three concluding."
		chunks := translator.SplitIntoChunks(text, 50)
		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
		}
		joined := translator.JoinChunks(chunks)
		if !strings.Contains(joined, "Paragraph one") || !strings.Contains(joined, "Paragraph three") {
			t.Errorf("joined paragraphs mismatch: %s", joined)
		}
	})
}
func TestLongSlideAndTableKeepBoundaries(t *testing.T) {
	for _, text := range []string{
		"## Slide 1\n\n" + strings.Repeat("A rational agent perceives the environment. ", 400),
		"## Slide 1\n\n| Agent | Sensor |\n|---|---|\n" + strings.Repeat("| Medical diagnosis system | Touchscreen |\n", 200),
	} {
		chunks := translator.SplitIntoChunks(text, 2500)
		if len(chunks) < 2 {
			t.Fatal("oversized slide was not split")
		}
		for _, chunk := range chunks {
			if len(chunk.Text) > 2500 {
				t.Fatalf("oversized chunk: %d", len(chunk.Text))
			}
		}
		if got := translator.JoinChunks(chunks); got != strings.TrimSpace(text) {
			t.Fatal("chunking changed paragraph/table separators")
		}
	}
}
