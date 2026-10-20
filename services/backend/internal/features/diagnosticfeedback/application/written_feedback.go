package application

import (
	"strconv"
	"strings"
	"unicode"
)

// Words that turn a completed review into a new task: imperatives and
// «нужно/следует … сделать» requests addressed to the student.
var (
	requestWords = wordSet("давайте добавьте дополните сформулируйте опишите проанализируйте перечислите вспомните запишите повторите объясните назовите выберите сравните приведите расскажите ответьте уточните проверьте определите изучите пересмотрите разберите закрепите попробуйте обратите укажите продолжите продолжайте выполните подумайте обоснуйте посчитайте рассчитайте докажите решите отправьте перейдите")
	modalWords   = wordSet("нужно необходимо следует стоит можете")
	actionWords  = wordSet("добавить дополнить сформулировать описать перечислить записать указать определить сравнить пояснить рассказать уточнить объяснить ответить назвать выбрать обосновать привести решить посчитать")
)

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// narrative reports whether the text explains rather than asks: no question
// and no request to do something.
func narrative(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "?") {
		return false
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) })
	for i, word := range words {
		if requestWords[word] {
			return false
		}
		if modalWords[word] {
			next := i + 1
			if next < len(words) && words[next] == "вам" {
				next++
			}
			if next < len(words) && actionWords[words[next]] {
				return false
			}
		}
	}
	return true
}

// applyWrittenFeedback places the model prose that passes the checks; the rest
// keeps its current text. Findings, scores and plan targets stay as decided by
// AnalyzeResult. It reports whether every finding and the summary were written.
func applyWrittenFeedback(report *DeterministicReport, written WrittenFeedback) bool {
	complete := true
	if summary := strings.TrimSpace(written.Summary); narrative(summary) {
		report.Summary = summary
	} else {
		complete = false
	}
	prose := map[string]string{}
	seen := map[string]bool{}
	for _, detail := range written.Details {
		text := strings.TrimSpace(detail.Text)
		if seen[detail.ID] {
			delete(prose, detail.ID)
			continue
		}
		seen[detail.ID] = true
		if narrative(text) {
			prose[detail.ID] = text
		}
	}
	for _, evidence := range report.Evidence {
		text, ok := prose[evidence.ID]
		if !ok {
			complete = false
			continue
		}
		if index, found := strings.CutPrefix(evidence.ID, "strength:"); found {
			if i, err := strconv.Atoi(index); err == nil && i >= 0 && i < len(report.Strengths) {
				report.Strengths[i] = text
			}
			continue
		}
		setTopicText(report, evidence, text)
	}
	return complete
}
