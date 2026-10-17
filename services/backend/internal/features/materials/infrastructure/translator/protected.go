package translator

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var immutableSpans = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~|`[^`\\n]*`|\\$\\$.*?\\$\\$|\\$[^$\\n]*\\$|\\\\\\[.*?\\\\\\]|\\\\\\(.*?\\\\\\)|<!--.*?-->")
var protectedTokens = regexp.MustCompile(`__COURSE_KEEP_[a-f0-9]+_\d+__`)

type protectedSpan struct{ token, text string }

func protectMarkdown(text string) (string, []protectedSpan) {
	digest := sha256.Sum256([]byte(text))
	var spans []protectedSpan
	masked := immutableSpans.ReplaceAllStringFunc(text, func(value string) string {
		token := fmt.Sprintf("__COURSE_KEEP_%x_%d__", digest[:6], len(spans))
		spans = append(spans, protectedSpan{token, value})
		return token
	})
	return masked, spans
}

func restoreMarkdown(text string, spans []protectedSpan) (string, error) {
	previous := -1
	for _, span := range spans {
		if strings.Count(text, span.token) != 1 {
			return "", fmt.Errorf("translation lost or duplicated protected fragment %s", span.token)
		}
		position := strings.Index(text, span.token)
		if position <= previous {
			return "", fmt.Errorf("translation reordered protected fragments")
		}
		previous = position
	}
	for _, span := range spans {
		text = strings.Replace(text, span.token, span.text, 1)
	}
	return text, nil
}

func untranslatedProse(text string) bool {
	// Identifiers and OCR image markers are data, not English prose.
	clean := immutableSpans.ReplaceAllString(text, " ")
	clean = protectedTokens.ReplaceAllString(clean, " ")
	if NeedsTranslation(clean) {
		return true
	}
	for _, paragraph := range strings.Split(clean, "\n\n") {
		if NeedsTranslation(paragraph) {
			return true
		}
	}
	return false
}

func hasSignificantEnglish(text string) bool {
	clean := immutableSpans.ReplaceAllString(text, " ")
	clean = protectedTokens.ReplaceAllString(clean, " ")
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
	if total > 0 && float64(cyrillic)/float64(total) >= 0.50 {
		return false
	}

	return untranslatedProse(text)
}

func sameTableShape(source, translated string) bool {
	shape := func(text string) []int {
		var result []int
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
				continue
			}
			pipes, escaped := 0, false
			for _, char := range line {
				if char == '|' && !escaped {
					pipes++
				}
				if char == '\\' {
					escaped = !escaped
				} else {
					escaped = false
				}
			}
			result = append(result, pipes)
		}
		return result
	}
	return slices.Equal(shape(source), shape(translated))
}
