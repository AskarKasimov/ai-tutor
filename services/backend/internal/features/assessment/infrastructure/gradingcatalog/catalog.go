package gradingcatalog

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

//go:embed competency_map.json
var defaultCatalog []byte

type fileFormat struct {
	Tasks []application.GradingContext `json:"tasks"`
}

type Catalog struct {
	byID map[string]application.GradingContext
}

func New() (*Catalog, error) {
	return NewFromBytes(defaultCatalog)
}

func NewFromBytes(data []byte) (*Catalog, error) {
	var source fileFormat
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return nil, fmt.Errorf("decode grading catalog: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode grading catalog: trailing JSON data")
		}
		return nil, fmt.Errorf("decode grading catalog: trailing data: %w", err)
	}
	if len(source.Tasks) == 0 {
		return nil, fmt.Errorf("grading catalog has no tasks")
	}

	catalog := &Catalog{byID: make(map[string]application.GradingContext, len(source.Tasks))}
	for i, task := range source.Tasks {
		normalize(&task)
		if err := validate(task); err != nil {
			return nil, fmt.Errorf("task %d: %w", i+1, err)
		}
		if _, exists := catalog.byID[task.TaskID]; exists {
			return nil, fmt.Errorf("duplicate task_id %q", task.TaskID)
		}
		catalog.byID[task.TaskID] = task
	}
	return catalog, nil
}

func (c *Catalog) ContextForTask(_ context.Context, taskID string) (application.GradingContext, error) {
	gradingContext, ok := c.byID[strings.TrimSpace(taskID)]
	if !ok {
		return application.GradingContext{}, fault.New(
			fault.NotFound,
			"GRADING_CONTEXT_NOT_FOUND",
			"Контекст для оценивания задания не найден.",
		)
	}
	return clone(gradingContext), nil
}

func clone(gradingContext application.GradingContext) application.GradingContext {
	gradingContext.Options = append([]string(nil), gradingContext.Options...)
	gradingContext.Criteria = append([]application.Criterion(nil), gradingContext.Criteria...)
	return gradingContext
}

func normalize(task *application.GradingContext) {
	task.TaskID = strings.TrimSpace(task.TaskID)
	task.Question = strings.TrimSpace(task.Question)
	task.VoiceInstruction = strings.TrimSpace(task.VoiceInstruction)
	task.ReferenceAnswer = strings.TrimSpace(task.ReferenceAnswer)
	task.Outcome.Title = strings.TrimSpace(task.Outcome.Title)
	task.Outcome.Taxonomy = strings.TrimSpace(task.Outcome.Taxonomy)
	task.Outcome.Level = strings.TrimSpace(task.Outcome.Level)
	task.MaterialContext.Knowledge = strings.TrimSpace(task.MaterialContext.Knowledge)
	task.MaterialContext.Skills = strings.TrimSpace(task.MaterialContext.Skills)
	for i := range task.Options {
		task.Options[i] = strings.TrimSpace(task.Options[i])
	}
	for i := range task.Criteria {
		task.Criteria[i].Key = strings.TrimSpace(task.Criteria[i].Key)
		task.Criteria[i].Description = strings.TrimSpace(task.Criteria[i].Description)
	}
}

func validate(task application.GradingContext) error {
	required := []struct {
		name  string
		value string
	}{
		{"task_id", task.TaskID},
		{"question", task.Question},
		{"voice_instruction", task.VoiceInstruction},
		{"reference_answer", task.ReferenceAnswer},
		{"outcome.title", task.Outcome.Title},
		{"outcome.taxonomy", task.Outcome.Taxonomy},
		{"outcome.level", task.Outcome.Level},
		{"material_context.knowledge", task.MaterialContext.Knowledge},
		{"material_context.skills", task.MaterialContext.Skills},
	}
	for _, field := range required {
		if field.value == "" {
			return fmt.Errorf("%s is empty", field.name)
		}
	}
	for i, option := range task.Options {
		if option == "" {
			return fmt.Errorf("options[%d] is empty", i)
		}
	}
	if len(task.Criteria) == 0 {
		return fmt.Errorf("criteria is empty")
	}
	seen := make(map[string]struct{}, len(task.Criteria))
	for i, criterion := range task.Criteria {
		if criterion.Key == "" {
			return fmt.Errorf("criteria[%d].key is empty", i)
		}
		if criterion.Description == "" {
			return fmt.Errorf("criteria[%d].description is empty", i)
		}
		if _, exists := seen[criterion.Key]; exists {
			return fmt.Errorf("duplicate criterion key %q", criterion.Key)
		}
		seen[criterion.Key] = struct{}{}
	}
	return nil
}
