package modelapi

import (
	"regexp"
	"strings"
	"unicode"
)

var codeSpans = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")
var escapedParagraphs = regexp.MustCompile(`(?:\\r\\n|\\n){2,}`)

var mathSpans = regexp.MustCompile(`(?s)\$\$.*?\$\$|\$[^$\n]*\$`)

// russianProse reports whether prose outside code and formulas is mainly
// Cyrillic; English terms inside Russian text are fine.
func russianProse(text string) bool {
	prose := mathSpans.ReplaceAllString(codeSpans.ReplaceAllString(text, " "), " ")
	cyrillic, latin := 0, 0
	for _, r := range prose {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case r < unicode.MaxASCII && unicode.IsLetter(r):
			latin++
		}
	}
	return cyrillic > 0 && cyrillic >= latin
}

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
