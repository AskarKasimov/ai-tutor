// Package application implements training-task generation policy: it loads the
// existing tasks of one educational outcome (topic) from the competency map as
// grounding samples and asks a generator for new analogous training tasks.
package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const (
	MinCount     = 1
	MaxCount     = 5
	DefaultCount = 3
)

// Task is a single task (an existing sample or a generated analogue).
type Task struct {
	Question         string
	Criteria         string
	VoiceInstruction string
	Options          []string
}

// Source groups the existing tasks of one outcome together with its place in
// the competency map, so the generator can stay on topic.
type Source struct {
	OutcomeID       string
	OutcomeName     string
	ConstituentName string
	CompetencyName  string
	Tasks           []Task
}

// Outcome is a selectable topic of the imported competency map.
type Outcome struct {
	ID              string
	Name            string
	ConstituentName string
	CompetencyName  string
	TaskCount       int
}

// Result is the generated training set for one outcome.
type Result struct {
	OutcomeID   string
	OutcomeName string
	Tasks       []Task
}

// SourceRepository reads grounding samples and the topic catalog from the map.
type SourceRepository interface {
	SourceByOutcome(context.Context, string) (Source, error)
	ListOutcomes(context.Context) ([]Outcome, error)
}

// Generator produces new training tasks analogous to the given samples.
type Generator interface {
	Generate(context.Context, Source, int) ([]Task, error)
}

type Service struct {
	sources   SourceRepository
	generator Generator
}

func New(sources SourceRepository, generator Generator) *Service {
	return &Service{sources: sources, generator: generator}
}

func validText(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// Outcomes lists topics available for training generation.
func (s *Service) Outcomes(ctx context.Context) ([]Outcome, error) {
	return s.sources.ListOutcomes(ctx)
}

// Generate validates the request, loads the outcome samples and returns a
// validated set of new training tasks. Generated tasks are not persisted: like
// the prototype assessment endpoint, the result is returned on the fly.
func (s *Service) Generate(ctx context.Context, outcomeID string, count int) (Result, error) {
	if !validText(outcomeID, 128) {
		return Result{}, fault.Validation("outcome_id", "Выберите тему (ОР).")
	}
	if count == 0 {
		count = DefaultCount
	}
	if count < MinCount || count > MaxCount {
		return Result{}, fault.Validation("count", "Допускается от 1 до 5 заданий.")
	}
	source, err := s.sources.SourceByOutcome(ctx, outcomeID)
	if err != nil {
		return Result{}, err
	}
	if len(source.Tasks) == 0 {
		return Result{}, fault.New(fault.NotFound, "OUTCOME_TASKS_EMPTY", "У выбранной темы нет заданий, по которым можно построить аналогию.")
	}
	generated, err := s.generator.Generate(ctx, source, count)
	if err != nil {
		return Result{}, err
	}
	tasks, err := validateTasks(generated, count, source.Tasks)
	if err != nil {
		return Result{}, err
	}
	return Result{OutcomeID: source.OutcomeID, OutcomeName: source.OutcomeName, Tasks: tasks}, nil
}

// validateTasks rejects model output that is malformed, off-size or repeats a
// known task. It also trims the stored text fields.
func validateTasks(tasks []Task, want int, source []Task) ([]Task, error) {
	if len(tasks) != want {
		return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула неожиданное число заданий.")
	}
	seen := map[string]bool{}
	for _, sample := range source {
		seen[normalize(sample.Question)] = true
	}
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if !validText(task.Question, 5000) {
			return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректный текст задания.")
		}
		if !validText(task.Criteria, 2000) {
			return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректные критерии задания.")
		}
		if !validText(task.VoiceInstruction, 500) {
			return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректную голосовую инструкцию.")
		}
		if len(task.Options) > 12 {
			return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула слишком много вариантов ответа.")
		}
		for _, option := range task.Options {
			if !validText(option, 500) {
				return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректный вариант ответа.")
			}
		}
		key := normalize(task.Question)
		if seen[key] {
			return nil, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель повторила уже известное задание.")
		}
		seen[key] = true
		task.Question = strings.TrimSpace(task.Question)
		task.Criteria = strings.TrimSpace(task.Criteria)
		task.VoiceInstruction = strings.TrimSpace(task.VoiceInstruction)
		out = append(out, task)
	}
	return out, nil
}
