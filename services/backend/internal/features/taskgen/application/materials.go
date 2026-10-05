package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type MaterialInput struct {
	Name       string
	Content    string
	OutcomeIDs []string
}

type MaterialRepository interface {
	ImportMaterial(context.Context, string, []string, []string, int64) error
}

type MaterialService struct {
	repository MaterialRepository
	now        func() int64
}

func NewMaterialService(repository MaterialRepository, now func() int64) *MaterialService {
	return &MaterialService{repository: repository, now: now}
}

func (s *MaterialService) Import(ctx context.Context, input MaterialInput) error {
	if strings.TrimSpace(input.Name) == "" || utf8.RuneCountInString(input.Name) > 200 {
		return fault.Validation("name", "Название материала должно содержать 1–200 символов.")
	}
	if strings.TrimSpace(input.Content) == "" || !utf8.ValidString(input.Content) || utf8.RuneCountInString(input.Content) > 100000 {
		return fault.Validation("content", "Текст материала должен содержать до 100 000 символов.")
	}
	if len(input.OutcomeIDs) == 0 || len(input.OutcomeIDs) > 100 {
		return fault.Validation("outcome_ids", "Укажите от 1 до 100 образовательных результатов.")
	}
	seen := make(map[string]struct{}, len(input.OutcomeIDs))
	outcomeIDs := make([]string, 0, len(input.OutcomeIDs))
	for _, id := range input.OutcomeIDs {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 128 {
			return fault.Validation("outcome_ids", "Идентификатор образовательного результата некорректен.")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		outcomeIDs = append(outcomeIDs, id)
	}
	return s.repository.ImportMaterial(ctx, strings.TrimSpace(input.Name), outcomeIDs, splitChunks(input.Content, 1200), s.now())
}

func splitChunks(text string, maxRunes int) []string {
	words := strings.Fields(text)
	chunks := []string{}
	for _, word := range words {
		wordRunes := []rune(word)
		for len(wordRunes) > maxRunes {
			chunks = append(chunks, string(wordRunes[:maxRunes]))
			wordRunes = wordRunes[maxRunes:]
		}
		fragment := string(wordRunes)
		if len(chunks) == 0 || len([]rune(chunks[len(chunks)-1]))+1+len(wordRunes) > maxRunes {
			chunks = append(chunks, fragment)
		} else {
			last := len(chunks) - 1
			chunks[last] += " " + fragment
		}
	}
	return chunks
}
