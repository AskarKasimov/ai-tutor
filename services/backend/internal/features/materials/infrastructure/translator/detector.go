package translator

import (
	"regexp"
	"unicode"
)

var (
	codeSpans    = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")
	mathSpans    = regexp.MustCompile(`(?s)\$\$.*?\$\$|\$[^$\n]*\$`)
	slideHeaders = regexp.MustCompile(`(?m)^##\s+(?:Слайд|Slide)\s+\d+.*$`)
	urlSpans     = regexp.MustCompile(`https?://\S+`)
)

// DetectLanguage inspects prose outside code blocks, LaTeX formulas, URLs and
// slide markers to determine whether the text is predominantly English and
// needs translation to Russian.
func DetectLanguage(markdown string) (lang string, needsTranslation bool) {
	clean := codeSpans.ReplaceAllString(markdown, " ")
	clean = mathSpans.ReplaceAllString(clean, " ")
	clean = slideHeaders.ReplaceAllString(clean, " ")
	clean = urlSpans.ReplaceAllString(clean, " ")

	cyrillic := 0
	latin := 0

	for _, r := range clean {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case r < unicode.MaxASCII && unicode.IsLetter(r):
			latin++
		}
	}

	total := cyrillic + latin
	if total < 30 {
		return "unknown", false
	}

	// If Latin letters make up more than 65% of the prose and total Latin count is substantial
	if latin > 40 && float64(latin)/float64(total) > 0.65 {
		return "en", true
	}

	return "ru", false
}

// NeedsTranslation reports whether the text is predominantly English.
func NeedsTranslation(markdown string) bool {
	_, needs := DetectLanguage(markdown)
	return needs
}
