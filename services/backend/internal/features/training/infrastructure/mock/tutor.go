package mock

import (
	"context"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
)

type Tutor struct{}

func (Tutor) Reply(ctx context.Context, input training.TutorContext) (training.TutorReply, error) {
	if err := ctx.Err(); err != nil {
		return training.TutorReply{}, err
	}
	if input.Check != nil {
		if input.Check.Score == input.Check.MaxScore {
			return training.TutorReply{AssistantText: "Вы сформулировали ответ и связали его с условием. Хотите проверить ещё какой-нибудь признак?", Pedagogy: training.PedagogyConfirmation}, nil
		}
		return training.TutorReply{AssistantText: "Пока не все требования выполнены. Какой признак из условия поможет уточнить вашу формулировку?", Pedagogy: training.PedagogyHint}, nil
	}
	text := strings.ToLower(strings.TrimSpace(input.StudentText))
	if text == "" {
		return training.TutorReply{AssistantText: "Что уже понятно в условии и какой результат нужно получить?", Pedagogy: training.PedagogyQuestion}, nil
	}
	if strings.Contains(text, "не знаю") || strings.Contains(text, "дай ответ") || strings.Contains(text, "покажи решение") {
		return training.TutorReply{AssistantText: "Давайте начнём с одного шага: какой результат требуется получить для одного объекта?", Pedagogy: training.PedagogyHint}, nil
	}
	if strings.Contains(text, "почему") || strings.Contains(text, "что такое") {
		return training.TutorReply{AssistantText: "Сначала выделите, что именно должно получиться на выходе. Это число или одна из заданных групп?", Pedagogy: training.PedagogyConcept}, nil
	}
	ready := strings.Contains(text, "потому") || strings.Contains(text, "так как") || strings.Contains(text, "выбирает один из классов")
	return training.TutorReply{AnswerReady: ready, AssistantText: "Вы назвали важную часть рассуждения. Какое свойство результата подтверждает ваш выбор?", Pedagogy: training.PedagogyQuestion}, nil
}

var _ interface {
	Reply(context.Context, training.TutorContext) (training.TutorReply, error)
} = Tutor{}
