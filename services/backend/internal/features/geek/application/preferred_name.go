package application

import (
	"regexp"
	"strings"
)

// Accept only a single name, never arbitrary instructions, as prompt data.
var nameRequest = regexp.MustCompile(`(?i)(?:меня зовут|зови меня|называй меня|обращайся ко мне)[\s,:]+([\p{L}][\p{L}\-]{0,39})`)

func preferredName(text string) (string, bool) {
	all := nameRequest.FindAllStringSubmatchIndex(text, -1)
	if len(all) == 0 {
		return "", false
	}
	loc := all[len(all)-1]
	name := text[loc[2]:loc[3]]
	switch strings.ToLower(name) {
	case "в", "на", "по", "так", "нейтрально", "мужском", "женском":
		return "", false
	}
	remaining := nameRequest.ReplaceAllString(text, " ")
	words := strings.FieldsFunc(strings.ToLower(remaining), func(r rune) bool { return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' || r == '\n' })
	for _, word := range words {
		if word != strings.ToLower(name) && word != "пожалуйста" {
			return name, false
		}
	}
	return name, true
}
