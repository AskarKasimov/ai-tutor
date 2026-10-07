package application

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const PromptVersion = "task-generation-v1"

type Outcome struct {
	ID, Name, CompetencyName, ConstituentName  string
	IncludeInTest                              *bool
	TaxonomyCode, ALDLevelCode, TopicLevelCode *string
	Importance                                 *int16
	EducationalContent                         *string
	CurriculumSections                         []CurriculumSection
}

type CurriculumSection struct {
	Code                   string
	Title                  string
	CurriculumCompetencies []string
}

type Example struct {
	ID, Question                                string
	Options                                     []string
	VoiceInstruction, ReferenceAnswer, Criteria *string
}

type MaterialChunk struct {
	ID, Name, Content string
}

type Context struct {
	Revision  int64
	Outcome   Outcome
	Examples  []Example
	Materials []MaterialChunk
}

type Draft struct {
	Question         string
	Options          []string
	VoiceInstruction string
	ReferenceAnswer  string
	Criteria         string
}

type Generator interface {
	Generate(context.Context, Context) (Draft, error)
}

type Repository interface {
	GetByKey(context.Context, string, string) (Task, error)
	Context(context.Context, string) (Context, error)
	Persist(context.Context, Context, string, string, string, Draft, int64) (Task, error)
}

type Task struct {
	ID, OutcomeID, Question, Origin             string
	Options                                     []string
	VoiceInstruction, ReferenceAnswer, Criteria *string
}

type Service struct {
	repository Repository
	generator  Generator
	model      string
	now        func() int64
}

func New(repository Repository, generator Generator, model string, now func() int64) *Service {
	return &Service{repository: repository, generator: generator, model: model, now: now}
}

func validText(value string, max int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}

func (s *Service) Generate(ctx context.Context, outcomeID, requestKey, requestedBy string) (Task, error) {
	if !validText(outcomeID, 128) || !validText(requestKey, 128) || !validText(requestedBy, 128) {
		return Task{}, fault.Validation("idempotency_key", "Укажите ключ идемпотентности и идентификатор ОР.")
	}
	if task, err := s.repository.GetByKey(ctx, outcomeID, requestKey); err == nil {
		return task, nil
	} else if !isNotFound(err) {
		return Task{}, err
	}
	snapshot, err := s.repository.Context(ctx, outcomeID)
	if err != nil {
		return Task{}, err
	}
	draft, err := s.generator.Generate(ctx, snapshot)
	if err != nil {
		return Task{}, err
	}
	if !validText(draft.Question, 5000) || !validText(draft.VoiceInstruction, 500) || !validText(draft.ReferenceAnswer, 2000) || len(draft.Options) > 12 {
		return Task{}, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректное задание.")
	}
	for _, option := range draft.Options {
		if !validText(option, 500) {
			return Task{}, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректный вариант ответа.")
		}
	}
	if draft.Criteria != "" && !validText(draft.Criteria, 2000) {
		return Task{}, fault.New(fault.Upstream, "TASK_GENERATION_FAILED", "Модель вернула некорректные критерии.")
	}
	if draft.Options == nil {
		draft.Options = []string{}
	}
	return s.repository.Persist(ctx, snapshot, requestKey, requestedBy, s.model, draft, s.now())
}

func isNotFound(err error) bool {
	var failure *fault.Error
	return errors.As(err, &failure) && failure.Kind == fault.NotFound
}
