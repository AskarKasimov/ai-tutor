package application

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"

const feedbackLeadIn = "Давай разберём ответ."
const pauseAcknowledgement = "Хорошо, подожду. Продолжай, когда будет удобно."

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
	return []string{feedbackLeadIn, pauseAcknowledgement,
		addressAcknowledgement(exam.AddressFeminine), addressAcknowledgement(exam.AddressMasculine), addressAcknowledgement(exam.AddressNeutral)}
}
