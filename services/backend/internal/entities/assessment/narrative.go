package assessment

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Words that turn a completed review into a new task: imperatives and
// «нужно/следует … сделать» requests addressed to the student.
var (
	requestWords = wordSet(`давайте добавьте дополните сформулируйте опишите проанализируйте перечислите вспомните запишите повторите объясните поясните назовите выберите сравните приведите расскажите ответьте уточните проверьте определите изучите пересмотрите рассмотрите разберите закрепите попробуйте обратите укажите продолжите продолжайте выполните подумайте обоснуйте посчитайте рассчитайте докажите решите отправьте перейдите
		добавь дополни сформулируй опиши проанализируй перечисли вспомни запиши повтори объясни поясни назови выбери сравни приведи расскажи ответь уточни проверь определи изучи пересмотри рассмотри разбери закрепи попробуй обрати укажи продолжи выполни подумай обоснуй посчитай рассчитай докажи реши`)
	modalWords  = wordSet("нужно необходимо следует стоит можете надо должны должен должна")
	addressees  = wordSet("вам тебе вы ты")
	actionWords = wordSet("добавить дополнить сформулировать описать перечислить записать указать определить сравнить пояснить рассказать уточнить объяснить ответить назвать выбрать обосновать привести решить посчитать рассчитать рассмотреть повторить изучить разобрать вспомнить проанализировать закрепить обратить попробовать проверить выполнить подумать доказать пересмотреть")
)

// modalReach is how far a request verb may stand from «нужно/следует»:
// «Вам нужно сначала объяснить выбор».
const modalReach = 3

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// Narrative reports whether the text explains rather than asks: no question
// and no request to do something.
func Narrative(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || strings.Contains(text, "?") {
		return false
	}
	for _, sentence := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return strings.ContainsRune(".!;:…\n", r) }) {
		words := strings.FieldsFunc(sentence, func(r rune) bool { return !unicode.IsLetter(r) })
		// A sentence opening with a «-ите/-йте» verb addresses the student:
		// «Поясните…», «Рассмотрите…».
		if len(words) > 0 && utf8.RuneCountInString(words[0]) > 4 &&
			(strings.HasSuffix(words[0], "ите") || strings.HasSuffix(words[0], "йте")) {
			return false
		}
		for i, word := range words {
			if requestWords[word] {
				return false
			}
			if !modalWords[word] {
				continue
			}
			addressed := i > 0 && addressees[words[i-1]]
			for _, next := range words[i+1 : min(len(words), i+1+modalReach)] {
				addressed = addressed || addressees[next]
				if actionWords[next] || (addressed && strings.HasSuffix(next, "ть")) {
					return false
				}
			}
		}
	}
	return true
}
