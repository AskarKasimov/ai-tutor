// Package mock is the offline teacher of the mock API mode and of tests.
package mock

import (
	"context"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
)

// Teacher writes fixed lessons. An answer is wrong when it says "не знаю".
type Teacher struct{}

func (Teacher) Theory(_ context.Context, input learning.LessonInput) (string, error) {
	return "Разберём «" + input.Item.OutcomeName + "». Это демонстрационная теория: здесь наставник объясняет главное по теме.", nil
}

func (Teacher) Practice(_ context.Context, input learning.LessonInput, _ string) (string, error) {
	return "Объясните своими словами, как вы применили бы «" + input.Item.OutcomeName + "» на практике?", nil
}

func (Teacher) Check(_ context.Context, input learning.CheckInput) (learning.Check, error) {
	if !strings.Contains(strings.ToLower(input.Answer), "не знаю") {
		return learning.Check{Correct: true, Feedback: "Верно! Вы уловили суть, идём дальше."}, nil
	}
	check := learning.Check{Feedback: "Пока не получилось. Давайте разберём ещё раз: начните с определения и приведите пример."}
	if !input.LastAttempt {
		check.Question = "Попробуйте ещё раз: приведите пример по теме «" + input.Item.OutcomeName + "»."
	}
	return check, nil
}
