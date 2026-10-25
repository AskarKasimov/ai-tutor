package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type tutorTaskContext struct {
	Question           string   `json:"question"`
	Options            []string `json:"options,omitempty"`
	VoiceInstruction   string   `json:"voice_instruction"`
	OutcomeName        string   `json:"outcome_name"`
	TaxonomyCode       string   `json:"taxonomy_code,omitempty"`
	ALDLevelCode       string   `json:"ald_level_code,omitempty"`
	EducationalContent string   `json:"educational_content,omitempty"`
	ReferenceAnswer    string   `json:"reference_answer"`
	Criteria           string   `json:"criteria,omitempty"`
}

func contextTooLarge() error {
	return fault.New(fault.Invalid, "TUTOR_CONTEXT_TOO_LARGE", "Условие задания не помещается в контекст репетитора.")
}

func taskContext(exercise training.Exercise) tutorTaskContext {
	task := exercise.Task
	return tutorTaskContext{
		Question: task.Question, Options: append([]string(nil), task.Options...), VoiceInstruction: task.VoiceInstruction,
		OutcomeName: task.OutcomeName, TaxonomyCode: task.TaxonomyCode, ALDLevelCode: task.ALDLevelCode,
		EducationalContent: task.EducationalContent, ReferenceAnswer: task.ReferenceAnswer, Criteria: task.Criteria,
	}
}

func validateContextLimits(limits training.ContextLimits) error {
	if limits.TaskMaxChars < 1 || limits.HistoryMaxChars < 1 || limits.SummaryMaxChars < 1 {
		return fault.Validation("context_limits", "Лимиты контекста должны быть положительными.")
	}
	return nil
}

func cloneCheck(value *training.Check) *training.Check {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.CriterionResults = append([]training.CriterionResult(nil), value.CriterionResults...)
	for i := range cloned.CriterionResults {
		cloned.CriterionResults[i].Explanation = ""
	}
	cloned.Feedback = nil
	if value.TranscriptionID != nil {
		id := *value.TranscriptionID
		cloned.TranscriptionID = &id
	}
	return &cloned
}

func messageChars(message training.ChatMessage) int {
	return utf8.RuneCountInString(message.Text)
}

func historySummary(history []training.ChatMessage, recentCount, maxChars int) string {
	if recentCount >= len(history) || maxChars <= 0 {
		return ""
	}
	lines := make([]string, 0)
	used := 0
	for i := len(history) - recentCount - 1; i >= 0; i-- {
		message := history[i]
		if message.Role != training.ChatRoleUser || message.Kind == training.ChatKindWelcome {
			continue
		}
		excerpt := strings.Join(strings.Fields(message.Text), " ")
		runes := []rune(excerpt)
		if len(runes) > 160 {
			excerpt = string(runes[:160])
		}
		line := fmt.Sprintf("%d: %s", message.Sequence, excerpt)
		remaining := maxChars - used
		if remaining <= 0 {
			break
		}
		if utf8.RuneCountInString(line) > remaining {
			lineRunes := []rune(line)
			if remaining < 8 {
				break
			}
			line = string(lineRunes[:remaining])
		}
		lines = append(lines, line)
		used += utf8.RuneCountInString(line) + 1
	}
	for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
		lines[left], lines[right] = lines[right], lines[left]
	}
	return strings.Join(lines, "\n")
}

func keepRecentSummary(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(value), "\n")
	kept := make([]string, 0, len(lines))
	used := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		remaining := maxChars - used
		if remaining <= 0 {
			break
		}
		count := utf8.RuneCountInString(line)
		if count > remaining {
			runes := []rune(line)
			if remaining < 8 {
				break
			}
			line = string(runes[:remaining])
			count = remaining
		}
		kept = append(kept, line)
		used += count + 1
	}
	for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
		kept[left], kept[right] = kept[right], kept[left]
	}
	return strings.Join(kept, "\n")
}

// BuildTutorContext bounds model context while retaining the complete journal in storage.
// It only copies user excerpts into the summary; no model-generated claims about the learner are added.
func BuildTutorContext(exercise training.Exercise, history []training.ChatMessage, input training.TutorInput, limits training.ContextLimits) (training.TutorContext, error) {
	if err := validateContextLimits(limits); err != nil {
		return training.TutorContext{}, err
	}
	encodedTask, err := json.Marshal(taskContext(exercise))
	if err != nil {
		return training.TutorContext{}, err
	}
	if utf8.RuneCount(encodedTask) > limits.TaskMaxChars {
		return training.TutorContext{}, contextTooLarge()
	}
	validHistory := make([]training.ChatMessage, 0, len(history))
	for _, message := range history {
		if message.Role != training.ChatRoleUser && message.Role != training.ChatRoleAssistant {
			continue
		}
		validHistory = append(validHistory, message)
	}
	start := max(0, len(validHistory)-24)
	recent := append([]training.ChatMessage(nil), validHistory[start:]...)
	chars := 0
	for _, message := range recent {
		chars += messageChars(message)
	}
	for len(recent) > 1 && chars > limits.HistoryMaxChars {
		chars -= messageChars(recent[0])
		recent = recent[1:]
		start++
	}
	if chars > limits.HistoryMaxChars {
		return training.TutorContext{}, contextTooLarge()
	}
	cloned := append([]training.ChatMessage(nil), recent...)
	summary := strings.TrimSpace(input.Summary)
	derivedSummary := historySummary(validHistory, len(cloned), limits.SummaryMaxChars)
	if derivedSummary != "" {
		if summary != "" {
			summary += "\n"
		}
		summary += derivedSummary
	}
	return training.TutorContext{
		Exercise: exercise, Summary: keepRecentSummary(summary, limits.SummaryMaxChars),
		Recent: cloned, StudentText: input.Text, Kind: input.Kind, Check: cloneCheck(input.Check),
	}, nil
}
