package application

import (
	"strings"
	"unicode"
)

// ReplySpeechParts keeps sentences intact where possible. Very long sentences
// are divided at clause boundaries, then words, to bound time to first speech.
func ReplySpeechParts(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	parts := []string{}
	start := 0
	add := func(block string) {
		block = strings.TrimSpace(block)
		for len([]rune(block)) > 180 {
			r := []rune(block)
			cut := 0
			for i := 179; i >= 90; i-- {
				if strings.ContainsRune(",;:—", r[i]) {
					cut = i + 1
					break
				}
			}
			if cut == 0 {
				break
			}
			parts = append(parts, strings.TrimSpace(string(r[:cut])))
			block = strings.TrimSpace(string(r[cut:]))
		}
		parts = append(parts, SplitSpeech(block, 180)...)
	}
	for i, ch := range runes {
		if !strings.ContainsRune(".!?…", ch) {
			continue
		}
		end := i + 1
		for end < len(runes) && strings.ContainsRune("\"»”)", runes[end]) {
			end++
		}
		if end < len(runes) && !unicode.IsSpace(runes[end]) {
			continue
		}
		word := i
		for word > start && !unicode.IsSpace(runes[word-1]) {
			word--
		}
		token := strings.ToLower(string(runes[word : i+1]))
		if ch == '.' && (i-word == 1 || token == "т.е." || token == "т.д." || token == "т.п." || token == "др." || token == "например.") {
			continue
		}
		add(string(runes[start:end]))
		start = end
	}
	add(string(runes[start:]))
	return parts
}
