// Package application runs the personal learning plan: it builds the plan from
// a completed diagnostic and the student's hours, opens lessons and checks
// practice answers.
package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Diagnostics interface {
	Get(ctx context.Context, owner, id string) (diagnostic.Session, error)
}

type Voice interface {
	Transcribe(ctx context.Context, owner string, audio []byte, mediaType string) (transcription.Transcription, error)
}

// Teacher writes lessons and checks practice answers.
type Teacher interface {
	Lesson(ctx context.Context, input learning.LessonInput) (learning.Lesson, error)
	Check(ctx context.Context, input learning.CheckInput) (learning.Check, error)
}

type IDs interface {
	New(prefix string) (string, error)
}

// Repository stores plans and their lesson chats. Update locks the plan, lets
// change apply its rules and appends the returned messages atomically.
type Repository interface {
	Outcomes(ctx context.Context, competencyIDs []string) ([]learning.Outcome, error)
	ByDiagnostic(ctx context.Context, owner, diagnosticID string) (learning.Plan, error)
	Get(ctx context.Context, owner, id string) (learning.Plan, error)
	Create(ctx context.Context, plan learning.Plan) (learning.Plan, error)
	Messages(ctx context.Context, planID string, item int) ([]learning.Message, error)
	Update(ctx context.Context, owner, id string, change func(*learning.Plan) ([]learning.Message, error)) (learning.Plan, error)
}

// MaxHours bounds the time a student can plan.
const MaxHours = 100

type Service struct {
	repo        Repository
	diagnostics Diagnostics
	voice       Voice
	teacher     Teacher
	ids         IDs
	now         func() time.Time
}

func New(repo Repository, diagnostics Diagnostics, voice Voice, teacher Teacher, ids IDs, now func() time.Time) *Service {
	return &Service{repo: repo, diagnostics: diagnostics, voice: voice, teacher: teacher, ids: ids, now: now}
}

func (s *Service) Get(ctx context.Context, owner, id string) (learning.Plan, error) {
	return s.repo.Get(ctx, owner, id)
}

func (s *Service) ByDiagnostic(ctx context.Context, owner, diagnosticID string) (learning.Plan, error) {
	return s.repo.ByDiagnostic(ctx, owner, diagnosticID)
}

func (s *Service) Messages(ctx context.Context, owner, id string, item int) ([]learning.Message, error) {
	plan, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if item < 0 || item >= len(plan.Items) {
		return nil, fault.New(fault.NotFound, "LEARNING_ITEM_NOT_FOUND", "Тема плана не найдена.")
	}
	return s.repo.Messages(ctx, plan.ID, item)
}

// Create builds the plan once per diagnostic; a repeated call returns it.
func (s *Service) Create(ctx context.Context, owner, diagnosticID string, hours int) (learning.Plan, error) {
	if hours < 1 || hours > MaxHours {
		return learning.Plan{}, fault.Validation("hours", "Укажите от 1 до 100 часов.")
	}
	if existing, err := s.repo.ByDiagnostic(ctx, owner, diagnosticID); err == nil {
		return existing, nil
	} else if !isNotFound(err) {
		return learning.Plan{}, err
	}
	session, err := s.diagnostics.Get(ctx, owner, diagnosticID)
	if err != nil {
		return learning.Plan{}, err
	}
	if session.Status != diagnostic.StatusCompleted {
		return learning.Plan{}, fault.New(fault.Conflict, "DIAGNOSTIC_SESSION_ACTIVE", "Сначала завершите диагностику.")
	}
	outcomes, err := s.outcomes(ctx, session)
	if err != nil {
		return learning.Plan{}, err
	}
	id, err := s.ids.New("learning")
	if err != nil {
		return learning.Plan{}, err
	}
	items, detail := BuildItems(outcomes, hours)
	plan := learning.Plan{
		ID: id, OwnerID: owner, DiagnosticID: diagnosticID,
		SubjectID: session.Variant.SubjectID, SubjectName: session.Variant.SubjectNameSnapshot,
		Hours: hours, Detail: detail, Items: items, Current: -1, CreatedAt: s.now().Unix(),
	}
	if next := plan.Next(); next >= 0 {
		plan.Items[next].Status = learning.ItemCurrent
		plan.Current = next
	}
	return s.repo.Create(ctx, plan)
}

// outcomes lists every outcome of each competency with a mistake in the
// diagnostic (its main task not answered in full), in variant order. A
// competency the current map no longer has falls back to the outcomes the
// diagnostic itself asked.
func (s *Service) outcomes(ctx context.Context, session diagnostic.Session) ([]learning.Outcome, error) {
	mains := map[string]int{}
	for _, answer := range session.Answers {
		if answer.Role == "main" {
			mains[answer.CompetencyID] = answer.Score
		}
	}
	failed := []string{}
	for _, competency := range session.Variant.Competencies {
		if score, answered := mains[competency.ID]; answered && score < 2 {
			failed = append(failed, competency.ID)
		}
	}
	if len(failed) == 0 {
		return nil, nil
	}
	fromMap, err := s.repo.Outcomes(ctx, failed)
	if err != nil {
		return nil, err
	}
	byCompetency := map[string][]learning.Outcome{}
	for _, outcome := range fromMap {
		byCompetency[outcome.CompetencyID] = append(byCompetency[outcome.CompetencyID], outcome)
	}
	result := []learning.Outcome{}
	for _, competency := range session.Variant.Competencies {
		if score, answered := mains[competency.ID]; !answered || score >= 2 {
			continue
		}
		if outcomes := byCompetency[competency.ID]; len(outcomes) > 0 {
			result = append(result, outcomes...)
			continue
		}
		seen := map[string]bool{}
		for _, task := range competency.Tasks {
			if seen[task.OutcomeID] {
				continue
			}
			seen[task.OutcomeID] = true
			result = append(result, learning.Outcome{
				CompetencyID: competency.ID, CompetencyName: competency.Name,
				OutcomeID: task.OutcomeID, OutcomeName: task.OutcomeName,
				TaxonomyCode: task.TaxonomyCode, Importance: int(task.Importance),
				EducationalContent: task.EducationalContent,
			})
		}
	}
	return result, nil
}

// BuildItems fits the outcomes into the hours: the most important ones first
// until the planned minutes are used up; the rest stay in the plan as
// "if time remains". Fewer hours than needed make theory brief, plenty of
// hours make it deep. At least one outcome is always in the plan.
func BuildItems(outcomes []learning.Outcome, hours int) ([]learning.Item, learning.Detail) {
	items := make([]learning.Item, len(outcomes))
	total := 0
	for i, outcome := range outcomes {
		theory, practice := learning.DefaultTheoryMinutes, learning.DefaultPracticeMinutes
		if outcome.TheoryMinutes != nil {
			theory = *outcome.TheoryMinutes
		}
		if outcome.PracticeMinutes != nil {
			practice = *outcome.PracticeMinutes
		}
		items[i] = learning.Item{
			CompetencyID: outcome.CompetencyID, CompetencyName: outcome.CompetencyName,
			OutcomeID: outcome.OutcomeID, OutcomeName: outcome.OutcomeName,
			TaxonomyCode: outcome.TaxonomyCode, Importance: outcome.Importance,
			EducationalContent: outcome.EducationalContent,
			TheoryMinutes:      theory, PracticeMinutes: practice, Status: learning.ItemPending,
		}
		total += theory + practice
	}
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return items[order[a]].Importance > items[order[b]].Importance })
	budget, used, selected := hours*60, 0, 0
	for _, i := range order {
		if used+items[i].Minutes() <= budget || selected == 0 {
			items[i].InPlan = true
			used += items[i].Minutes()
			selected++
		}
	}
	switch {
	case selected < len(items):
		return items, learning.DetailBrief
	case total > 0 && budget >= 2*total:
		return items, learning.DetailDeep
	default:
		return items, learning.DetailStandard
	}
}

// Open writes the lesson of the current item once: theory and the first
// practice question. A lesson already written is left as it is.
func (s *Service) Open(ctx context.Context, owner, id string) (learning.Plan, error) {
	plan, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return learning.Plan{}, err
	}
	if plan.Completed() || plan.Items[plan.Current].Question != "" {
		return plan, nil
	}
	index := plan.Current
	if s.teacher == nil {
		return learning.Plan{}, fault.New(fault.Unavailable, "TEACHER_UNAVAILABLE", "Наставник временно недоступен.")
	}
	lesson, err := s.teacher.Lesson(ctx, learning.LessonInput{SubjectName: plan.SubjectName, Item: plan.Items[index], Detail: plan.Detail})
	if err != nil {
		return learning.Plan{}, err
	}
	now := s.now().Unix()
	return s.repo.Update(ctx, owner, id, func(plan *learning.Plan) ([]learning.Message, error) {
		// Another request may have opened the lesson meanwhile.
		if plan.Current != index || plan.Items[index].Question != "" {
			return nil, nil
		}
		plan.Items[index].Question = lesson.Question
		return []learning.Message{
			{ItemIndex: index, Role: learning.RoleAssistant, Kind: learning.KindTheory, Text: lesson.Theory, CreatedAt: now},
			{ItemIndex: index, Role: learning.RoleAssistant, Kind: learning.KindPractice, Text: lesson.Question, CreatedAt: now},
		}, nil
	})
}

var errAnswerPending = fault.New(fault.Conflict, "LEARNING_ANSWER_PENDING", "Предыдущий ответ ещё проверяется.")

// Answer transcribes a spoken practice answer, saves it right away so the chat
// can show it, then checks it.
func (s *Service) Answer(ctx context.Context, owner, id string, audio []byte, mediaType string) (learning.Plan, error) {
	plan, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return learning.Plan{}, err
	}
	if plan.Completed() {
		return learning.Plan{}, fault.New(fault.Conflict, "LEARNING_PLAN_COMPLETED", "План обучения уже пройден.")
	}
	if plan.PendingAnswer != nil {
		return learning.Plan{}, errAnswerPending
	}
	index := plan.Current
	if plan.Items[index].Question == "" {
		return learning.Plan{}, fault.New(fault.Conflict, "LEARNING_LESSON_NOT_OPEN", "Сначала откройте урок.")
	}
	if s.voice == nil {
		return learning.Plan{}, fault.New(fault.Unavailable, "VOICE_UNAVAILABLE", "Распознавание речи временно недоступно.")
	}
	transcribed, err := s.voice.Transcribe(ctx, owner, audio, mediaType)
	if err != nil {
		return learning.Plan{}, err
	}
	text := strings.TrimSpace(transcribed.Text)
	if text == "" {
		return learning.Plan{}, fault.New(fault.Invalid, "NO_SPEECH_DETECTED", "Не удалось распознать ответ. Запишите его ещё раз.")
	}
	now := s.now().Unix()
	if _, err = s.repo.Update(ctx, owner, id, func(plan *learning.Plan) ([]learning.Message, error) {
		if plan.Current != index || plan.PendingAnswer != nil {
			return nil, errAnswerPending
		}
		plan.PendingAnswer = &text
		return []learning.Message{{ItemIndex: index, Role: learning.RoleUser, Kind: learning.KindAnswer, Text: text, CreatedAt: now}}, nil
	}); err != nil {
		return learning.Plan{}, err
	}
	return s.check(ctx, owner, id)
}

// Retry checks a saved answer again after the check failed.
func (s *Service) Retry(ctx context.Context, owner, id string) (learning.Plan, error) {
	plan, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return learning.Plan{}, err
	}
	if plan.PendingAnswer == nil {
		return learning.Plan{}, fault.New(fault.Conflict, "LEARNING_NO_PENDING_ANSWER", "Нет ответа, ожидающего проверки.")
	}
	return s.check(ctx, owner, id)
}

// check evaluates the pending answer. A correct answer finishes the item; a
// wrong one gets an explanation built on the student's answer and a new
// question, until the attempts are used up and the item is left for review.
func (s *Service) check(ctx context.Context, owner, id string) (learning.Plan, error) {
	plan, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return learning.Plan{}, err
	}
	if plan.PendingAnswer == nil || plan.Completed() {
		return plan, nil
	}
	if s.teacher == nil {
		return learning.Plan{}, fault.New(fault.Unavailable, "TEACHER_UNAVAILABLE", "Наставник временно недоступен.")
	}
	index, answer := plan.Current, *plan.PendingAnswer
	item := plan.Items[index]
	messages, err := s.repo.Messages(ctx, plan.ID, index)
	if err != nil {
		return learning.Plan{}, err
	}
	var theory string
	for _, message := range messages {
		if message.Role == learning.RoleAssistant && message.Kind == learning.KindTheory {
			theory = message.Text
			break
		}
	}
	result, err := s.teacher.Check(ctx, learning.CheckInput{
		SubjectName: plan.SubjectName, Item: item, Detail: plan.Detail,
		Question: item.Question, Answer: answer, Theory: theory, LastAttempt: item.Attempts+1 >= learning.MaxAttempts,
	})
	if err != nil {
		return learning.Plan{}, err
	}
	now := s.now().Unix()
	return s.repo.Update(ctx, owner, id, func(plan *learning.Plan) ([]learning.Message, error) {
		if plan.Current != index || plan.PendingAnswer == nil || *plan.PendingAnswer != answer {
			return nil, errAnswerPending
		}
		plan.PendingAnswer = nil
		current := &plan.Items[index]
		current.Attempts++
		messages := []learning.Message{{ItemIndex: index, Role: learning.RoleAssistant, Kind: learning.KindFeedback, Text: result.Feedback, CreatedAt: now}}
		switch {
		case result.Correct:
			current.Status = learning.ItemPassed
		case current.Attempts >= learning.MaxAttempts || result.Question == "":
			current.Status = learning.ItemReview
		default:
			current.Question = result.Question
			messages = append(messages, learning.Message{ItemIndex: index, Role: learning.RoleAssistant, Kind: learning.KindPractice, Text: result.Question, CreatedAt: now})
			return messages, nil
		}
		plan.Current = plan.Next()
		if plan.Current >= 0 {
			plan.Items[plan.Current].Status = learning.ItemCurrent
		}
		return messages, nil
	})
}

func isNotFound(err error) bool {
	var f *fault.Error
	return errors.As(err, &f) && f.Kind == fault.NotFound
}
