// Copy into the original translator package to reproduce preservation gaps.
// These tests document baseline behavior; they are NOT acceptance tests.
package translator

import (
	"context"
	"strings"
	"testing"
)

type qaBrokenCompleter struct{}

func (qaBrokenCompleter) Complete(context.Context, string, string) (string, error) {
	return `{"translation":"Краткое резюме без источника и формулы."}`, nil
}

func TestQAProbeCorruptTranslationAccepted(t *testing.T) {
	input := "[D01 | original.pdf | с.1 | core]\nThe learning algorithm minimizes the loss function in every iteration. $x=2$"
	out, err := NewWithCompleter(qaBrokenCompleter{}, 1).Translate(context.Background(), input)
	if err != nil || strings.Contains(out, "[D01") || strings.Contains(out, "$x=2$") {
		t.Fatal("baseline changed", out, err)
	}
	t.Log("Confirmed: missing source marker, formula and most content accepted without validation")
}

func TestQAProbeShortEnglishSkipped(t *testing.T) {
	if NeedsTranslation("Machine learning") {
		t.Fatal("baseline changed")
	}
	t.Log("Confirmed: short English text is skipped")
}

func TestQAProbeCodeFenceSplit(t *testing.T) {
	input := "Introduction\n\n```python\n# " + strings.Repeat("example ", 30) + "\nx = 2\n```\n\nConclusion"
	chunks := SplitIntoChunks(input, 100)
	split := false
	for _, c := range chunks {
		if strings.Count(c.Text, "```")%2 != 0 {
			split = true
		}
	}
	if !split {
		t.Fatal("baseline changed")
	}
	t.Log("Confirmed: a Markdown-style heading inside code splits the code fence")
}

func TestQAProbeOversizeSection(t *testing.T) {
	chunks := SplitIntoChunks("## Slide 1\n"+strings.Repeat("Long educational text. ", 1000), 2500)
	if len(chunks) != 1 || len(chunks[0].Text) <= 2500 {
		t.Fatal("baseline changed")
	}
	t.Log("Confirmed: oversized section bypasses the chunk target")
}
