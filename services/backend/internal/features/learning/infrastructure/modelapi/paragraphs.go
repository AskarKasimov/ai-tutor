package modelapi

import (
	"regexp"
	"strings"
)

var codeSpans = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")
var escapedParagraphs = regexp.MustCompile(`(?:\\r\\n|\\n){2,}`)

// Normalize only paragraph separators in prose after decoding model JSON.
// Code escapes and LaTeX commands such as \nu and \nabla remain unchanged.
func normalizeTheoryParagraphs(text string) string {
	var result strings.Builder
	start := 0
	for _, span := range codeSpans.FindAllStringIndex(text, -1) {
		result.WriteString(escapedParagraphs.ReplaceAllString(text[start:span[0]], "\n\n"))
		result.WriteString(text[span[0]:span[1]])
		start = span[1]
	}
	result.WriteString(escapedParagraphs.ReplaceAllString(text[start:], "\n\n"))
	return result.String()
}
