package variant

import (
	"strings"
	"unicode/utf8"
)

// ValidTaskContent matches the content limits enforced when a diagnostic
// snapshot is created.
func ValidTaskContent(question string, options []string, voiceInstruction, referenceAnswer string) bool {
	if !ValidTaskText(question, 5000) || !ValidTaskText(voiceInstruction, 500) || !ValidTaskText(referenceAnswer, 2000) || len(options) > 12 {
		return false
	}
	for _, option := range options {
		if !ValidTaskText(option, 500) {
			return false
		}
	}
	return true
}

func ValidTaskText(value string, maxRunes int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && strings.IndexByte(value, 0) < 0 && utf8.RuneCountInString(value) <= maxRunes
}
