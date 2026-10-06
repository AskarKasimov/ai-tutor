package application

import (
	"context"
	"strings"
	"unicode"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
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

func (s *MaterialService) Import(ctx context.Context, actor user.User, input MaterialInput) error {
	if err := s.Authorize(actor); err != nil {
		return err
	}
	if !validText(input.Name, 200) {
		return fault.Validation("name", "Название материала должно содержать 1–200 символов.")
	}
	if !validText(input.Content, 100000) {
		return fault.Validation("content", "Текст материала должен содержать до 100 000 символов.")
	}
	if len(input.OutcomeIDs) == 0 || len(input.OutcomeIDs) > 100 {
		return fault.Validation("outcome_ids", "Укажите от 1 до 100 образовательных результатов.")
	}
	seen := make(map[string]struct{}, len(input.OutcomeIDs))
	outcomeIDs := make([]string, 0, len(input.OutcomeIDs))
	for _, id := range input.OutcomeIDs {
		id = strings.TrimSpace(id)
		if !validText(id, 128) || len(id) > 128 {
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

// Authorize allows transports to reject unauthorized uploads before reading them.
func (s *MaterialService) Authorize(actor user.User) error {
	if actor.Role != user.Admin {
		return fault.New(fault.Forbidden, "FORBIDDEN", "Операция доступна только admin.")
	}
	return nil
}

func splitChunks(text string, maxRunes int) []string {
	runes := []rune(text)
	chunks := []string{}
	leadingWhitespace := ""
	for start := 0; start < len(runes); {
		end := min(start+maxRunes, len(runes))
		if end < len(runes) {
			// Prefer a word boundary, keeping the separator in its original position.
			for i := end; i > start; i-- {
				if unicode.IsSpace(runes[i-1]) {
					end = i
					break
				}
			}
		}
		chunk := string(runes[start:end])
		start = end
		if strings.TrimSpace(chunk) == "" {
			// Keep whitespace with adjacent content instead of persisting an empty chunk.
			// Long whitespace runs can make that chunk exceed the target size.
			if len(chunks) == 0 {
				leadingWhitespace += chunk
			} else {
				chunks[len(chunks)-1] += chunk
			}
			continue
		}
		chunks = append(chunks, leadingWhitespace+chunk)
		leadingWhitespace = ""
	}
	return chunks
}
