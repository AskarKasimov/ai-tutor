package application

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"

const feedbackLeadIn = "Давай разберём ответ."

var feedbackLeadIns = []string{feedbackLeadIn, "Давай посмотрим на твой ответ.", "Подведём итог твоему ответу.", "Посмотрим, что получилось."}
var nextQuestionTransitions = []string{"Давай перейдём к следующему вопросу.", "Теперь перейдём к следующей теме.", "Продолжим: следующий вопрос.", "Хорошо, перейдём дальше."}

// NextQuestionTransition is deterministic for existing turns and is only used
// when feedback closes a topic and a new main question follows it.
func NextQuestionTransition(turn exam.Turn) string {
	if turn.Speaker != exam.SpeakerGeek || turn.Kind != exam.KindQuestion || turn.Recap == "" {
		return ""
	}
	return nextQuestionTransitions[(turn.Line/2)%len(nextQuestionTransitions)]
}

const pauseAcknowledgement = "Хорошо, подожду. Продолжай, когда будет удобно."
const RepeatUnclearSpeech = "Возможно, запись оборвалась или распознался посторонний разговор. Повтори ответ или скажи «я не знаю»."
const ConfirmUnclearSpeech = "Возможно, запись оборвалась или распознался посторонний разговор. Перезаписать ответ?"
const AskUnclearPart = "Какая часть вопроса непонятна?"

func addressAcknowledgement(preference exam.AddressPreference) string {
	switch preference {
	case exam.AddressFeminine:
		return "Хорошо, дальше буду обращаться в женском роде. Можем продолжить с того же вопроса."
	case exam.AddressMasculine:
		return "Хорошо, дальше буду обращаться в мужском роде. Можем продолжить с того же вопроса."
	default:
		return "Хорошо, дальше буду использовать нейтральные обращения. Можем продолжить с того же вопроса."
	}
}

func ConversationSpeech() []string {
	texts := append([]string{}, feedbackLeadIns...)
	texts = append(texts, nextQuestionTransitions...)
	return append(texts, pauseAcknowledgement, RepeatUnclearSpeech, ConfirmUnclearSpeech, AskUnclearPart,
		addressAcknowledgement(exam.AddressFeminine), addressAcknowledgement(exam.AddressMasculine), addressAcknowledgement(exam.AddressNeutral))
}
