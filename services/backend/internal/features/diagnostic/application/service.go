package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Service struct {
	store    Store
	variants VariantReader
	voice    Voice
	grader   Grader
	ids      IDGenerator
	now      func() time.Time
}

func New(store Store, variants VariantReader, voice Voice, grader Grader, ids IDGenerator, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, variants: variants, voice: voice, grader: grader, ids: ids, now: now}
}

func (s *Service) Start(ctx context.Context, ownerID, variantID, key string) (diagnostic.Progress, bool, error) {
	if err := validKey(key); err != nil {
		return diagnostic.Progress{}, false, err
	}
	if !validID(variantID) {
		return diagnostic.Progress{}, false, fault.Validation("variant_id", "Укажите ID варианта.")
	}
	requestDigest := digest(variantID)
	if existing, found, err := s.store.FindStart(ctx, ownerID, key, requestDigest); err != nil {
		return diagnostic.Progress{}, false, err
	} else if found {
		return progress(existing), true, nil
	}
	value, err := s.variants.Get(ctx, ownerID, variantID)
	if err != nil {
		return diagnostic.Progress{}, false, err
	}
	if value.OwnerID != ownerID || value.ID != variantID {
		return diagnostic.Progress{}, false, fault.New(fault.NotFound, "VARIANT_NOT_FOUND", "Вариант не найден.")
	}
	snapshot, err := snapshotVariant(value)
	if err != nil {
		return diagnostic.Progress{}, false, err
	}
	id, err := s.ids.New("diagnostic")
	if err != nil {
		return diagnostic.Progress{}, false, err
	}
	session := diagnostic.Session{
		ID: id, OwnerID: ownerID, VariantID: variantID, Status: diagnostic.StatusActive,
		Variant:            snapshot,
		AcceptedRequests:   make(map[string]diagnostic.AcceptedRequest),
		StartRequestDigest: requestDigest,
	}
	stored, reused, err := s.store.Create(ctx, ownerID, key, session.StartRequestDigest, session)
	if err != nil {
		return diagnostic.Progress{}, false, err
	}
	return progress(stored), reused, nil
}

func (s *Service) Read(ctx context.Context, ownerID, sessionID string) (diagnostic.Progress, error) {
	if !validID(sessionID) {
		return diagnostic.Progress{}, fault.Validation("id", "Укажите корректный ID диагностической сессии.")
	}
	value, err := s.store.Get(ctx, ownerID, sessionID)
	if err != nil {
		return diagnostic.Progress{}, err
	}
	return progress(value), nil
}

func (s *Service) CurrentAudio(ctx context.Context, ownerID, sessionID string) ([]byte, error) {
	if !validID(sessionID) {
		return nil, fault.Validation("id", "Укажите корректный ID диагностической сессии.")
	}
	value, err := s.store.Get(ctx, ownerID, sessionID)
	if err != nil {
		return nil, err
	}
	if value.Status != diagnostic.StatusActive {
		return nil, fault.New(fault.Conflict, "DIAGNOSTIC_SESSION_COMPLETED", "Диагностическая сессия уже завершена.")
	}
	task := currentTask(value)
	if task == nil || strings.TrimSpace(task.VoiceInstruction) == "" {
		return nil, fault.New(fault.Upstream, "DIAGNOSTIC_INSTRUCTION_MISSING", "У текущего задания нет голосовой инструкции.")
	}
	return s.voice.Synthesize(ctx, task.VoiceInstruction)
}

func (s *Service) Answer(ctx context.Context, ownerID, sessionID, taskID, key string, data []byte, media string) (diagnostic.Progress, error) {
	if err := validKey(key); err != nil {
		return diagnostic.Progress{}, err
	}
	if !validID(sessionID) {
		return diagnostic.Progress{}, fault.Validation("id", "Укажите корректный ID диагностической сессии.")
	}
	if !validID(taskID) {
		return diagnostic.Progress{}, fault.Validation("variant_task_id", "Укажите ID текущей позиции варианта.")
	}
	if err := audio.Check(data, media); err != nil {
		return diagnostic.Progress{}, err
	}
	value, err := s.store.Get(ctx, ownerID, sessionID)
	if err != nil {
		return diagnostic.Progress{}, err
	}
	// Bind retries to the task and exact audio payload, independent of multipart boundaries.
	h := sha256.New()
	_, _ = h.Write([]byte(taskID + "\x00" + media + "\x00"))
	_, _ = h.Write(data)
	fingerprint := hex.EncodeToString(h.Sum(nil))
	token, err := s.ids.New("attempt")
	if err != nil {
		return diagnostic.Progress{}, err
	}
	accepted, reservation, err := s.store.Reserve(ctx, ownerID, sessionID, key, fingerprint, taskID, token)
	if err != nil {
		return diagnostic.Progress{}, err
	}
	if accepted != nil {
		return accepted.Response, nil
	}
	current := currentTask(value)
	if current == nil || current.ID != taskID {
		_ = s.store.Fail(context.Background(), ownerID, sessionID, token, "", "")
		return diagnostic.Progress{}, fault.New(fault.Conflict, "DIAGNOSTIC_TASK_NOT_CURRENT", "Укажите текущее задание сессии.")
	}
	transcriptionID, text := reservation.TranscriptionID, reservation.TranscriptionText
	if transcriptionID == "" {
		transcription, transcribeErr := s.voice.Transcribe(ctx, ownerID, data, media)
		if transcribeErr != nil {
			_ = s.store.Fail(context.Background(), ownerID, sessionID, token, "", "")
			return diagnostic.Progress{}, transcribeErr
		}
		if !validTranscription(transcription, ownerID) {
			_ = s.store.Fail(context.Background(), ownerID, sessionID, token, "", "")
			return diagnostic.Progress{}, fault.New(fault.Upstream, "TRANSCRIPTION_INVALID_RESPONSE", "Сервис распознавания вернул некорректную расшифровку.")
		}
		transcriptionID, text = transcription.ID, transcription.Text
	}
	evaluation, err := s.grader.Evaluate(ctx, ownerID, transcriptionID, value.Variant.ID, current.ID)
	if err != nil {
		_ = s.store.Fail(context.Background(), ownerID, sessionID, token, transcriptionID, text)
		return diagnostic.Progress{}, err
	}
	if err = validateEvaluation(evaluation, current.Role); err != nil {
		_ = s.store.Fail(context.Background(), ownerID, sessionID, token, transcriptionID, text)
		return diagnostic.Progress{}, err
	}
	score := evaluation.Score
	criteria := make([]diagnostic.CriterionResult, 0, len(evaluation.CriterionResults))
	for _, item := range evaluation.CriterionResults {
		criteria = append(criteria, diagnostic.CriterionResult{Key: item.Key, Satisfied: item.Satisfied, Explanation: item.Explanation})
	}
	answer := diagnostic.Answer{
		VariantTaskID: current.ID, SourceTaskID: current.SourceTaskID,
		CompetencyID: current.CompetencyID, OutcomeID: current.OutcomeID, Role: current.Role,
		Task:            *current,
		TranscriptionID: transcriptionID, Text: text, GraderScore: evaluation.Score, GraderMaxScore: evaluation.MaxScore,
		Score: score, Verdict: evaluation.Verdict,
		CriterionResults: criteria, Feedback: evaluation.Feedback,
		CreatedAt: s.now().Unix(),
	}
	transition := nextTransition(value, answer)
	result, err := s.store.Accept(ctx, ownerID, sessionID, token, key, fingerprint, answer, transition,
		value.Variant.IncludedCompetencyCount*3)
	if err != nil {
		_ = s.store.Fail(context.Background(), ownerID, sessionID, token, transcriptionID, text)
		return diagnostic.Progress{}, err
	}
	return result, nil
}

func (s *Service) Result(ctx context.Context, ownerID, sessionID string) (diagnostic.Result, error) {
	if !validID(sessionID) {
		return diagnostic.Result{}, fault.Validation("id", "Укажите корректный ID диагностической сессии.")
	}
	value, err := s.store.Get(ctx, ownerID, sessionID)
	if err != nil {
		return diagnostic.Result{}, err
	}
	if value.Status != diagnostic.StatusCompleted {
		return diagnostic.Result{}, fault.New(fault.Conflict, "DIAGNOSTIC_SESSION_ACTIVE", "Диагностическая сессия ещё не завершена.")
	}
	result := diagnostic.Result{
		SessionID: value.ID, Status: value.Status, VariantID: value.VariantID,
		MapRevision: value.Variant.MapRevision, IncludedCompetencyCount: value.Variant.IncludedCompetencyCount,
		SkippedCompetencies: append([]diagnostic.SkippedCompetency{}, value.Variant.SkippedCompetencies...), CompletedTasks: len(value.Answers),
		TotalTasks: value.Variant.IncludedCompetencyCount * 3, MaximumScore: value.Variant.IncludedCompetencyCount * 2,
		Answers: append([]diagnostic.Answer(nil), value.Answers...), UntestedBasics: append([]diagnostic.TaskSnapshot{}, value.SkippedBasics...),
	}
	for _, answer := range value.Answers {
		if answer.Role == "main" {
			result.DiagnosticScore += answer.Score
		}
	}
	return result, nil
}

func validateEvaluation(value assessment.Evaluation, role string) error {
	maxScore := 2
	if role == "basic" {
		maxScore = 1
	} else if role != "main" {
		return fault.New(fault.Upstream, "ASSESSMENT_INVALID_RESPONSE", "Задание содержит неизвестную роль.")
	}
	if value.MaxScore != maxScore || value.Score < 0 || value.Score > maxScore {
		return fault.New(fault.Upstream, "ASSESSMENT_INVALID_RESPONSE", "Грейдер вернул оценку вне диапазона роли задания.")
	}
	want := "incorrect"
	if value.Score == maxScore {
		want = "correct"
	} else if maxScore == 2 && value.Score == 1 {
		want = "partial"
	}
	if value.Verdict != want || len(value.CriterionResults) == 0 || len(value.Feedback) != 3 {
		return fault.New(fault.Upstream, "ASSESSMENT_INVALID_RESPONSE", "Грейдер вернул некорректную структуру оценки.")
	}
	for _, item := range value.CriterionResults {
		if strings.TrimSpace(item.Key) == "" || strings.TrimSpace(item.Explanation) == "" {
			return fault.New(fault.Upstream, "ASSESSMENT_INVALID_RESPONSE", "Грейдер вернул некорректные результаты критериев.")
		}
	}
	for _, item := range value.Feedback {
		if strings.TrimSpace(item) == "" || strings.ContainsAny(item, "\r\n") {
			return fault.New(fault.Upstream, "ASSESSMENT_INVALID_RESPONSE", "Грейдер вернул некорректный фидбэк.")
		}
	}
	return nil
}

func progress(value diagnostic.Session) diagnostic.Progress {
	result := diagnostic.Progress{SessionID: value.ID, Status: value.Status,
		Completed: len(value.Answers), Skipped: len(value.SkippedBasics), Total: value.Variant.IncludedCompetencyCount * 3}
	if task := currentTask(value); task != nil {
		result.Current = task
	}
	return result
}

func nextTransition(value diagnostic.Session, answer diagnostic.Answer) diagnostic.Transition {
	transition := diagnostic.Transition{
		CurrentCompetency: value.CurrentCompetency,
		CurrentTask:       value.CurrentTask,
		Status:            value.Status,
	}
	competency := value.Variant.Competencies[value.CurrentCompetency]
	if answer.Role == "main" && answer.Score == 2 {
		transition.SkippedBasics = append(transition.SkippedBasics, competency.Tasks[value.CurrentTask+1:]...)
		transition.CurrentCompetency++
		transition.CurrentTask = 0
	} else if value.CurrentTask+1 < len(competency.Tasks) {
		transition.CurrentTask++
	} else {
		transition.CurrentCompetency++
		transition.CurrentTask = 0
	}
	if transition.CurrentCompetency >= len(value.Variant.Competencies) {
		transition.Status = diagnostic.StatusCompleted
	}
	return transition
}

func currentTask(value diagnostic.Session) *diagnostic.TaskSnapshot {
	if value.CurrentCompetency >= len(value.Variant.Competencies) {
		return nil
	}
	competency := value.Variant.Competencies[value.CurrentCompetency]
	if value.CurrentTask >= len(competency.Tasks) {
		return nil
	}
	task := competency.Tasks[value.CurrentTask]
	return &task
}

func snapshotVariant(value variant.Variant) (diagnostic.VariantSnapshot, error) {
	if !validID(value.ID) || len(value.Competencies) == 0 || value.IncludedCompetencyCount != len(value.Competencies) {
		return diagnostic.VariantSnapshot{}, fault.New(fault.Invalid, "VARIANT_INVALID", "Вариант не содержит заданий для диагностики.")
	}
	snapshot := diagnostic.VariantSnapshot{
		ID: value.ID, MapRevision: value.MapRevision, AlgorithmVersion: value.AlgorithmVersion,
		IncludedCompetencyCount: value.IncludedCompetencyCount,
		SkippedCompetencies:     make([]diagnostic.SkippedCompetency, 0, len(value.SkippedCompetencies)),
		Competencies:            make([]diagnostic.Competency, 0, len(value.Competencies)),
	}
	for _, skipped := range value.SkippedCompetencies {
		snapshot.SkippedCompetencies = append(snapshot.SkippedCompetencies, diagnostic.SkippedCompetency{ID: skipped.CompetencyID, Name: skipped.CompetencyName, Code: skipped.Code})
	}
	for _, selection := range value.Competencies {
		if !validID(selection.Competency.ID) || strings.TrimSpace(selection.Competency.Name) == "" || len(selection.Tasks) != 3 {
			return diagnostic.VariantSnapshot{}, fault.New(fault.Invalid, "VARIANT_INVALID", "Компетенция варианта должна содержать три позиции.")
		}
		competency := diagnostic.Competency{ID: selection.Competency.ID, Name: selection.Competency.Name, Position: selection.Position, Tasks: make([]diagnostic.TaskSnapshot, 0, 3)}
		for i, item := range selection.Tasks {
			wantRole := "basic"
			if i == 0 {
				wantRole = "main"
			}
			if !validID(item.ID) || !validID(item.Task.ID) || item.Role != wantRole ||
				!validID(item.Task.Competency.ID) || item.Task.Competency.ID != selection.Competency.ID ||
				strings.TrimSpace(item.Task.Competency.Name) == "" || !validID(item.Task.Constituent.ID) ||
				strings.TrimSpace(item.Task.Constituent.Name) == "" || !validID(item.Task.Outcome.ID) ||
				strings.TrimSpace(item.Task.Outcome.Name) == "" || !validTaskText(item.Task.Question, 5000) ||
				item.Task.ReferenceAnswer == nil || !validTaskText(*item.Task.ReferenceAnswer, 2000) ||
				item.Task.VoiceInstruction == nil || !validTaskText(*item.Task.VoiceInstruction, 500) ||
				len(item.Task.Options) > 12 {
				return diagnostic.VariantSnapshot{}, fault.New(fault.Invalid, "VARIANT_INVALID", "Вариант содержит неполное или некорректное задание.")
			}
			for _, option := range item.Task.Options {
				if !validTaskText(option, 500) {
					return diagnostic.VariantSnapshot{}, fault.New(fault.Invalid, "VARIANT_INVALID", "Вариант содержит некорректный ответ.")
				}
			}
			voiceInstruction, reference, criteria := "", "", ""
			if item.Task.VoiceInstruction != nil {
				voiceInstruction = *item.Task.VoiceInstruction
			}
			if item.Task.ReferenceAnswer != nil {
				reference = *item.Task.ReferenceAnswer
			}
			if item.Task.Criteria != nil {
				criteria = *item.Task.Criteria
			}
			taxonomy, level, content := "", "", ""
			importance, included := int16(0), false
			if item.Task.Outcome.TaxonomyCode != nil {
				taxonomy = *item.Task.Outcome.TaxonomyCode
			}
			if item.Task.Outcome.ALDLevelCode != nil {
				level = *item.Task.Outcome.ALDLevelCode
			}
			if item.Task.Outcome.EducationalContent != nil {
				content = *item.Task.Outcome.EducationalContent
			}
			if item.Task.Outcome.Importance != nil {
				importance = *item.Task.Outcome.Importance
			}
			if item.Task.Outcome.IncludeInTest != nil {
				included = *item.Task.Outcome.IncludeInTest
			}
			options := append([]string{}, item.Task.Options...)
			competency.Tasks = append(competency.Tasks, diagnostic.TaskSnapshot{
				ID: item.ID, SourceTaskID: item.Task.ID, Role: item.Role,
				CompetencyID: item.Task.Competency.ID, CompetencyName: item.Task.Competency.Name,
				ConstituentID: item.Task.Constituent.ID, ConstituentName: item.Task.Constituent.Name,
				OutcomeID: item.Task.Outcome.ID, OutcomeName: item.Task.Outcome.Name,
				Question: item.Task.Question, Options: options,
				VoiceInstruction: voiceInstruction, ReferenceAnswer: reference, Criteria: criteria,
				TaxonomyCode: taxonomy, ALDLevelCode: level, Importance: importance,
				IncludeInTest: included, EducationalContent: content,
			})
		}
		snapshot.Competencies = append(snapshot.Competencies, competency)
	}
	return snapshot, nil
}

func validKey(key string) error {
	if len(key) < 1 || len(key) > 128 || strings.TrimSpace(key) != key || strings.IndexFunc(key, func(r rune) bool { return r < 0x21 || r > 0x7e }) >= 0 {
		return fault.Validation("Idempotency-Key", "Укажите ключ длиной от 1 до 128 печатных ASCII-символов без пробелов.")
	}
	return nil
}

func validID(value string) bool {
	if len(value) == 0 || len(value) > 128 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

func validTaskText(value string, maxRunes int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && strings.IndexByte(value, 0) < 0 && utf8.RuneCountInString(value) <= maxRunes
}

func validTranscription(value transcription.Transcription, ownerID string) bool {
	return validID(value.ID) && value.OwnerID == ownerID && validTaskText(value.Text, 20000)
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
