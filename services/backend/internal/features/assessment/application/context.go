package application

import "context"

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
