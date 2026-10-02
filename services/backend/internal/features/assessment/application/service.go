package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Task struct {
	Question         string
	Options          []string
	VoiceInstruction string
	CorrectAnswer    string
}

type Evaluation struct {
	Score    int      `json:"score"`
	Feedback []string `json:"feedback"`
}

type Transcriptions interface {
	TextByOwner(context.Context, string, string) (string, error)
}

type Grader interface {
	Grade(context.Context, Task, string) (Evaluation, error)
}

type Service struct {
	transcriptions Transcriptions
	grader         Grader
}

func New(transcriptions Transcriptions, grader Grader) *Service {
	return &Service{transcriptions: transcriptions, grader: grader}
}

func validText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}

func (s *Service) Evaluate(ctx context.Context, ownerID, transcriptionID string, task Task) (Evaluation, error) {
	if !validText(transcriptionID, 128) {
		return Evaluation{}, fault.Validation("transcription_id", "Укажите сохранённую расшифровку.")
	}
	if !validText(task.Question, 5000) {
		return Evaluation{}, fault.Validation("question", "Укажите текст задания до 5000 символов.")
	}
	if !validText(task.VoiceInstruction, 500) {
		return Evaluation{}, fault.Validation("voice_instruction", "Укажите голосовую инструкцию до 500 символов.")
	}
	if len(task.Options) > 12 {
		return Evaluation{}, fault.Validation("options", "Допускается не более 12 вариантов ответа.")
	}
	for _, option := range task.Options {
		if !validText(option, 500) {
			return Evaluation{}, fault.Validation("options", "Каждый вариант должен содержать 1–500 символов.")
		}
	}
	if task.CorrectAnswer != "" && !validText(task.CorrectAnswer, 2000) {
		return Evaluation{}, fault.Validation("correct_answer", "Эталонный ответ должен содержать не более 2000 символов.")
	}
	answer, err := s.transcriptions.TextByOwner(ctx, ownerID, transcriptionID)
	if err != nil {
		return Evaluation{}, err
	}
	if !validText(answer, 20000) {
		return Evaluation{}, fault.New(fault.Invalid, "INVALID_TRANSCRIPTION", "Расшифровка слишком длинная или повреждена.")
	}
	result, err := s.grader.Grade(ctx, task, answer)
	if err != nil {
		return Evaluation{}, err
	}
	if result.Score < 0 || result.Score > 2 || len(result.Feedback) != 3 {
		return Evaluation{}, fault.New(fault.Upstream, "ASSESSMENT_FAILED", "Модель вернула некорректную оценку.")
	}
	for i, line := range result.Feedback {
		line = strings.TrimSpace(line)
		if !validText(line, 240) || strings.ContainsAny(line, "\r\n") {
			return Evaluation{}, fault.New(fault.Upstream, "ASSESSMENT_FAILED", "Модель вернула некорректный фидбэк.")
		}
		result.Feedback[i] = line
	}
	return result, nil
}
