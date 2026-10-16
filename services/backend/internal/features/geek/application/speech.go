package application

import (
	"strings"
	"unicode/utf8"
)

// SplitSpeech cuts text into parts of at most limit runes for synthesis,
// preferring sentence ends, then spaces; a word longer than limit is cut.
func SplitSpeech(text string, limit int) []string {
	text = strings.TrimSpace(text)
	parts := []string{}
	for utf8.RuneCountInString(text) > limit {
		runes := []rune(text)
		cut := -1
		for i := limit - 1; i > limit/2; i-- {
			if strings.ContainsRune(".!?…", runes[i]) {
				cut = i + 1
				break
			}
		}
		if cut < 0 {
			for i := limit; i > limit/2; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
		}
		if cut < 0 {
			cut = limit
		}
		parts = append(parts, strings.TrimSpace(string(runes[:cut])))
		text = strings.TrimSpace(string(runes[cut:]))
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}
