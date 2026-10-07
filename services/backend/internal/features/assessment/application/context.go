package application

import (
	"context"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type OutcomeContext struct {
	Title    string `json:"title"`
	Taxonomy string `json:"taxonomy"`
	Level    string `json:"level"`
}

type Criterion struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

type CriterionResult struct {
	Key         string `json:"key"`
	Satisfied   bool   `json:"satisfied"`
	Explanation string `json:"explanation"`
}

type MaterialContext struct {
	Knowledge string `json:"knowledge"`
	Skills    string `json:"skills"`
}

type GradingContext struct {
	TaskID           string          `json:"task_id"`
	Role             string          `json:"role"`
	MaxScore         int             `json:"max_score"`
	Question         string          `json:"question"`
	Options          []string        `json:"options,omitempty"`
	VoiceInstruction string          `json:"voice_instruction"`
	ReferenceAnswer  string          `json:"reference_answer"`
	Outcome          OutcomeContext  `json:"outcome"`
	Criteria         []Criterion     `json:"criteria"`
	MaterialContext  MaterialContext `json:"material_context"`
}

type ContextProvider interface {
	ContextForTask(context.Context, string) (GradingContext, error)
}

func GradingContextFromVariantTask(item variant.VariantTask) (GradingContext, error) {
	task := item.Task
	question := strings.TrimSpace(task.Question)
	voiceInstruction := pointerText(task.VoiceInstruction)
	referenceAnswer := pointerText(task.ReferenceAnswer)
	if question == "" || voiceInstruction == "" || referenceAnswer == "" {
		return GradingContext{}, fault.New(
			fault.Invalid,
			"INVALID_GRADING_CONTEXT",
			"Снимок задания не содержит данных, необходимых для оценивания.",
		)
	}

	maxScore := 2
	switch item.Role {
	case "main":
	case "basic":
		maxScore = 1
	default:
		return GradingContext{}, fault.New(
			fault.Invalid,
			"INVALID_GRADING_CONTEXT",
			"Снимок задания содержит неизвестную роль.",
		)
	}

	correctness := "Ответ по смыслу соответствует эталонному ответу и предметному контексту."
	if criteria := pointerText(task.Criteria); criteria != "" {
		correctness = criteria
	}

	return GradingContext{
		TaskID:           item.ID,
		Role:             item.Role,
		MaxScore:         maxScore,
		Question:         question,
		Options:          append([]string(nil), task.Options...),
		VoiceInstruction: voiceInstruction,
		ReferenceAnswer:  referenceAnswer,
		Outcome: OutcomeContext{
			Title:    strings.TrimSpace(task.Outcome.Name),
			Taxonomy: pointerText(task.Outcome.TaxonomyCode),
			Level:    pointerText(task.Outcome.ALDLevelCode),
		},
		Criteria: []Criterion{
			{
				Key:         "answer_correctness",
				Description: correctness,
			},
			{
				Key:         "instruction_following",
				Description: "Ответ выполняет голосовую инструкцию: " + voiceInstruction,
			},
		},
		MaterialContext: MaterialContext{
			Knowledge: pointerText(task.Outcome.EducationalContent),
			Skills:    strings.TrimSpace(task.Constituent.Name),
		},
	}, nil
}

func pointerText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
