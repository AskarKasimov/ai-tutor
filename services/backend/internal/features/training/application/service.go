package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/transcription"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type Diagnostics interface {
	Get(context.Context, string, string) (diagnostic.Session, error)
}
type Voice interface {
	Transcribe(context.Context, string, []byte, string) (transcription.Transcription, error)
}
type Grader interface {
	EvaluateTraining(context.Context, string, string, training.Exercise) (assessment.Evaluation, error)
}
type IDs interface{ New(string) (string, error) }
type ExerciseProvider interface {
	Generate(context.Context, training.Target, []training.Attempt) (training.Task, string, error)
}
// TheoryWriter summarizes the ОС of a competency for the theory stage.
type TheoryWriter interface {
	Summarize(context.Context, training.TheoryRequest) (string, error)
}
type Repository interface {
	Get(context.Context, string, string) (training.Session, error)
	ByDiagnostic(context.Context, string, string) (training.Session, error)
	ByStartKey(context.Context, string, string, string) (training.Session, bool, error)
	Material(context.Context, string) (training.Material, error)
	Create(context.Context, training.Session, string, *int64) (training.Session, bool, error)
	// AdvanceTheory saves the state that follows the expected theory exercise.
	AdvanceTheory(context.Context, string, string, training.Session) (training.Progress, error)
	Reserve(context.Context, string, string, string, string, string, string) (*training.Progress, *training.Reservation, training.Session, error)
	Accept(context.Context, string, training.Reservation, training.Session, training.Attempt) (training.Progress, error)
	Fail(context.Context, string, string, training.Reservation) error
	ResetReservation(context.Context, string, string) error
	History(context.Context, string, string, int64, int) ([]training.Attempt, error)
}
type Service struct {
	repo        Repository
	diagnostics Diagnostics
	voice       Voice
	grader      Grader
	provider    ExerciseProvider
	ids         IDs
	now         func() time.Time
	audio       audioasset.MetadataReader
	regenerator audioasset.Regenerator
	checkedNext CheckedNextRepository
	theory      TheoryWriter
}

// WithTheory enables model summaries; without it theory shows the source ОС texts.
func (s *Service) WithTheory(writer TheoryWriter) *Service {
	s.theory = writer
	return s
}

func (s *Service) WithCheckedNext(repository CheckedNextRepository) *Service {
	s.checkedNext = repository
	return s
}

func New(repo Repository, diagnostics Diagnostics, voice Voice, grader Grader, provider ExerciseProvider, ids IDs, now func() time.Time) *Service {
	return &Service{repo: repo, diagnostics: diagnostics, voice: voice, grader: grader, provider: provider, ids: ids, now: now}
}
func (s *Service) WithAudio(reader audioasset.MetadataReader) *Service { s.audio = reader; return s }
func (s *Service) WithAudioRegenerator(regenerator audioasset.Regenerator) *Service {
	s.regenerator = regenerator
	return s
}
func ValidKey(key string) error {
	if key == "" || len(key) > 128 || strings.TrimSpace(key) != key || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return fault.Validation("Idempotency-Key", "Укажите ключ длиной от 1 до 128 символов.")
	}
	return nil
}
func (s *Service) plan(ctx context.Context, owner, id string) (training.Session, error) {
	d, err := s.diagnostics.Get(ctx, owner, id)
	if err != nil {
		return training.Session{}, err
	}
	if d.Status != diagnostic.StatusCompleted {
		return training.Session{}, fault.New(fault.Conflict, "DIAGNOSTIC_SESSION_ACTIVE", "Сначала завершите диагностику.")
	}
	state := training.Session{OwnerID: owner, DiagnosticID: id, SubjectID: d.Variant.SubjectID, SubjectName: d.Variant.SubjectNameSnapshot, Mode: "focused", PlanRevision: d.Variant.MapRevision, Round: 1, Targets: []training.Target{}}
	for _, a := range d.Answers {
		if a.Role == "main" {
			state.DiagnosticScore += a.Score
		}
	}
	state.MaximumScore = 2 * d.Variant.IncludedCompetencyCount
	// Each competency with a gap becomes one block; the shared classifier labels it.
	kinds := map[string]string{}
	for _, target := range diagnostic.TrainingTargets(d.Answers) {
		if _, exists := kinds[target.Answer.CompetencyID]; !exists {
			kinds[target.Answer.CompetencyID] = target.Kind
		}
	}
	excellent := len(kinds) == 0 && d.Variant.IncludedCompetencyCount > 0 && state.DiagnosticScore == state.MaximumScore
	if len(kinds) == 0 && !excellent {
		return state, nil
	}
	material, err := s.repo.Material(ctx, state.SubjectID)
	if err != nil {
		return state, err
	}
	if excellent {
		state.Mode = "free_practice"
		state.PlanRevision = material.Revision
	}
	for _, competency := range d.Variant.Competencies {
		kind, exists := kinds[competency.ID]
		if !exists {
			if !excellent {
				continue
			}
			kind = "free_practice"
		}
		if target, ok := blockTarget(kind, competency, d.Answers, material); ok {
			state.Targets = append(state.Targets, target)
		}
	}
	return state, nil
}

func trainingTask(t diagnostic.TaskSnapshot) training.Task {
	return training.Task{SourceTaskID: t.SourceTaskID, CompetencyID: t.CompetencyID, CompetencyName: t.CompetencyName, ConstituentID: t.ConstituentID, ConstituentName: t.ConstituentName, OutcomeID: t.OutcomeID, OutcomeName: t.OutcomeName, Question: t.Question, Options: append([]string{}, t.Options...), VoiceInstruction: t.VoiceInstruction, ReferenceAnswer: t.ReferenceAnswer, Criteria: t.Criteria, TaxonomyCode: t.TaxonomyCode, ALDLevelCode: t.ALDLevelCode, EducationalContent: t.EducationalContent, Importance: t.Importance, AudioAssetID: t.AudioAssetID}
}

// outcomeKey matches tasks by names because a reimport issues new IDs.
func outcomeKey(t training.Task) string { return t.CompetencyName + "\x00" + t.OutcomeName }

// taskPool puts other ready tasks of the same ОР first: the snapshot task was
// already shown in the diagnostic. Randomness is not needed; the pool rotates.
func taskPool(snapshot training.Task, bank []training.Task) []training.Task {
	pool := []training.Task{}
	for _, task := range bank {
		if outcomeKey(task) == outcomeKey(snapshot) && task.SourceTaskID != snapshot.SourceTaskID {
			pool = append(pool, task)
		}
	}
	return append(pool, snapshot)
}

func blockTarget(kind string, competency diagnostic.Competency, answers []diagnostic.Answer, material training.Material) (training.Target, bool) {
	var main *training.Task
	basics := []training.Task{}
	for _, snapshot := range competency.Tasks {
		task := trainingTask(snapshot)
		switch snapshot.Role {
		case "main":
			main = &task
		case "basic":
			basics = append(basics, task)
		}
	}
	if main == nil {
		return training.Target{}, false
	}
	block := &training.Block{Main: taskPool(*main, material.Tasks), Basics: [][]training.Task{}, BasicShown: make([]int, len(basics))}
	for _, basic := range basics {
		block.Basics = append(block.Basics, taskPool(basic, material.Tasks))
	}
	// Theory covers every ОС of the competency, including outcomes not in the test.
	block.Notes = append([]training.TheoryNote{}, material.Notes[competency.Name]...)
	if len(block.Notes) == 0 {
		for _, task := range append([]training.Task{*main}, basics...) {
			if strings.TrimSpace(task.EducationalContent) != "" {
				block.Notes = append(block.Notes, training.TheoryNote{OutcomeID: task.OutcomeID, OutcomeName: task.OutcomeName, IncludeInTest: true, Content: task.EducationalContent})
			}
		}
	}
	target := training.Target{Kind: kind, Source: *main, Block: block}
	for _, a := range answers {
		if a.CompetencyID != competency.ID || a.Role != "main" {
			continue
		}
		target.OriginalScore, target.OriginalMaxScore = a.Score, 2
		target.OriginalFeedback = append([]string{}, a.Feedback...)
		target.OriginalCriteria = []training.CriterionResult{}
		for _, c := range a.CriterionResults {
			target.OriginalCriteria = append(target.OriginalCriteria, training.CriterionResult{Key: c.Key, Satisfied: c.Satisfied, Explanation: c.Explanation})
		}
	}
	return target, true
}
func (s *Service) Preview(ctx context.Context, owner, id string) (training.Preview, error) {
	state, err := s.plan(ctx, owner, id)
	if err != nil {
		return training.Preview{}, err
	}
	p := training.Preview{DiagnosticID: id, SubjectID: state.SubjectID, SubjectName: state.SubjectName, Mode: state.Mode, PlanRevision: state.PlanRevision, DiagnosticScore: state.DiagnosticScore, MaximumScore: state.MaximumScore, Status: "ready", ConfirmedGaps: []training.TargetView{}, PartialCompetencies: []training.TargetView{}, Topics: []training.TargetView{}}
	for _, t := range state.Targets {
		v := training.View(t)
		p.Topics = append(p.Topics, v)
		switch t.Kind {
		case "confirmed_gap":
			p.ConfirmedGaps = append(p.ConfirmedGaps, v)
		case "partial_competency":
			p.PartialCompetencies = append(p.PartialCompetencies, v)
		}
	}
	if len(state.Targets) == 0 {
		p.Status = "no_practice_tasks"
	}
	return p, nil
}
func (s *Service) Start(ctx context.Context, owner, id, key string, revision *int64) (training.Progress, bool, error) {
	if err := ValidKey(key); err != nil {
		return training.Progress{}, false, err
	}
	if existing, found, err := s.repo.ByStartKey(ctx, owner, key, id); err != nil {
		return training.Progress{}, false, err
	} else if found {
		return existing.Progress(), true, nil
	}
	if existing, err := s.repo.ByDiagnostic(ctx, owner, id); err == nil {
		stored, _, err := s.repo.Create(ctx, existing, key, revision)
		return stored.Progress(), true, err
	} else {
		var f *fault.Error
		if !errors.As(err, &f) || f.Kind != fault.NotFound {
			return training.Progress{}, false, err
		}
	}
	state, err := s.plan(ctx, owner, id)
	if err != nil {
		return training.Progress{}, false, err
	}
	state.ID, err = s.ids.New("training")
	if err != nil {
		return training.Progress{}, false, err
	}
	if len(state.Targets) > 0 {
		state.Current, err = s.exercise(ctx, state, state.Entry(0), nil)
		if err != nil {
			return training.Progress{}, false, err
		}
	}
	stored, reused, err := s.repo.Create(ctx, state, key, revision)
	return stored.Progress(), reused, err
}

// exercise creates the instance for a step. It mutates the shared block state:
// pool counters and the cached theory are saved together with the new exercise.
func (s *Service) exercise(ctx context.Context, state training.Session, step training.Step, attempts []training.Attempt) (training.Exercise, error) {
	target := state.Targets[step.TargetIndex]
	block := target.Block
	exercise := training.Exercise{TargetIndex: step.TargetIndex, Round: state.Round, Stage: step.Stage, BasicIndex: step.BasicIndex}
	switch {
	case block == nil:
		if len(target.Sources) > 0 {
			target.Source = target.Sources[(state.Round-1)%int64(len(target.Sources))]
		}
	case step.Stage == training.StageTheory:
		exercise.Task = theoryTask(target.Source)
		exercise.ProviderVersion = "theory-v1"
		exercise.Theory = s.theoryText(ctx, state.SubjectName, target.Source, block)
		id, err := s.ids.New("exercise")
		exercise.ID = id
		return exercise, err
	case step.Stage == training.StageBasic:
		for len(block.BasicShown) < len(block.Basics) {
			block.BasicShown = append(block.BasicShown, 0)
		}
		if step.BasicIndex < 0 || step.BasicIndex >= len(block.Basics) {
			return training.Exercise{}, fault.New(fault.Conflict, "TRAINING_STEP_INVALID", "Некорректный шаг тренировки.")
		}
		task, ok := training.Pick(block.Basics[step.BasicIndex], block.BasicShown[step.BasicIndex])
		if !ok {
			return training.Exercise{}, fault.New(fault.Conflict, "TRAINING_STEP_INVALID", "Некорректный шаг тренировки.")
		}
		block.BasicShown[step.BasicIndex]++
		target.Source = task
	default:
		if task, ok := training.Pick(block.Main, block.MainShown); ok {
			target.Source = task
		}
		block.MainShown++
	}
	task, version, err := s.provider.Generate(ctx, target, attempts)
	if err != nil {
		return training.Exercise{}, err
	}
	exercise.Task, exercise.ProviderVersion = task, version
	exercise.ID, err = s.ids.New("exercise")
	return exercise, err
}

func theoryTask(main training.Task) training.Task {
	return training.Task{
		CompetencyID: main.CompetencyID, CompetencyName: main.CompetencyName,
		ConstituentID: main.ConstituentID, ConstituentName: main.ConstituentName,
		OutcomeID: main.OutcomeID, OutcomeName: main.OutcomeName,
		Question:         "Теория по теме «" + main.CompetencyName + "»",
		VoiceInstruction: "Прочитайте краткую теорию. Если что-то непонятно, спросите репетитора, затем переходите к закрепляющим задачам.",
		Options:          []string{},
	}
}

// theoryText generates the summary once per block. A model failure must not stop
// the student: the source ОС texts are shown and the next cycle retries the model.
func (s *Service) theoryText(ctx context.Context, subject string, main training.Task, block *training.Block) string {
	if strings.TrimSpace(block.Theory) != "" {
		return block.Theory
	}
	if s.theory != nil && len(block.Notes) > 0 {
		text, err := s.theory.Summarize(ctx, training.TheoryRequest{SubjectName: subject, CompetencyName: main.CompetencyName, MainOutcome: main.OutcomeName, Notes: append([]training.TheoryNote{}, block.Notes...)})
		if text = strings.TrimSpace(text); err == nil && text != "" {
			block.Theory = text
			return text
		}
	}
	return training.FallbackTheory(block.Notes)
}

// advance records the attempt and creates the exercise of the next step.
func (s *Service) advance(ctx context.Context, state *training.Session, attempt training.Attempt) error {
	score := attempt.Score
	state.Targets[state.Current.TargetIndex].LastScore = &score
	state.AnswerCount++
	step := state.Advance(attempt.Score)
	next, err := s.exercise(ctx, *state, step, []training.Attempt{attempt})
	if err != nil {
		return err
	}
	state.Current = next
	return nil
}

func theoryStage() error {
	return fault.New(fault.Conflict, "TRAINING_THEORY_STAGE", "Сейчас этап теории: изучите её и переходите к задачам.")
}

// CompleteTheory moves the student from the theory summary to the basic tasks.
func (s *Service) CompleteTheory(ctx context.Context, owner, id, exerciseID string) (training.Progress, error) {
	state, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return training.Progress{}, err
	}
	if state.Current.ID != exerciseID {
		return training.Progress{}, fault.New(fault.Conflict, "TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	if state.Current.Stage != training.StageTheory {
		return training.Progress{}, fault.New(fault.Conflict, "TRAINING_NOT_THEORY_STAGE", "Текущее упражнение не является теорией.")
	}
	step := state.Advance(0)
	next, err := s.exercise(ctx, state, step, nil)
	if err != nil {
		return training.Progress{}, err
	}
	state.Current = next
	return s.repo.AdvanceTheory(ctx, owner, exerciseID, state)
}
func (s *Service) Get(ctx context.Context, owner, id string) (training.Progress, error) {
	v, err := s.repo.Get(ctx, owner, id)
	return v.Progress(), err
}
func (s *Service) ByDiagnostic(ctx context.Context, owner, id string) (training.Progress, error) {
	v, err := s.repo.ByDiagnostic(ctx, owner, id)
	return v.Progress(), err
}
func (s *Service) Answer(ctx context.Context, owner, id, exerciseID, key string, data []byte, media string) (training.Progress, error) {
	return s.answerRequest(ctx, owner, id, exerciseID, key, data, media, false)
}

func (s *Service) Skip(ctx context.Context, owner, id, exerciseID, key string) (training.Progress, error) {
	return s.answerRequest(ctx, owner, id, exerciseID, key, nil, "", true)
}

// NextChecked advances only after a current explicit chat check. It shares the
// ordinary training exercise-generation policy while the chat repository owns
// the transaction that closes the old conversation and records the attempt.
func (s *Service) NextChecked(ctx context.Context, owner, id, exerciseID, key string, revision int64) (training.Progress, error) {
	if s.checkedNext == nil {
		return training.Progress{}, fault.New(fault.Unavailable, "TRAINING_NEXT_UNAVAILABLE", "Переход сейчас недоступен.")
	}
	if err := ValidKey(key); err != nil {
		return training.Progress{}, err
	}
	if revision < 0 {
		return training.Progress{}, fault.Validation("revision", "Ревизия должна быть неотрицательным целым числом.")
	}
	token, err := s.ids.New("training_next")
	if err != nil {
		return training.Progress{}, err
	}
	progress, reservation, err := s.checkedNext.ReserveNext(ctx, owner, id, exerciseID, key, revision, token)
	if err != nil {
		return training.Progress{}, err
	}
	if progress != nil {
		return *progress, nil
	}
	if reservation == nil {
		return training.Progress{}, fault.New(fault.Unavailable, "TRAINING_NEXT_UNAVAILABLE", "Переход сейчас недоступен.")
	}
	failed := true
	defer func() {
		if failed {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.checkedNext.FailNext(cleanupCtx, *reservation)
		}
	}()
	state := reservation.Session
	check := reservation.Check
	if state.Current.Stage == training.StageTheory {
		return training.Progress{}, theoryStage()
	}
	if state.Current.ID != exerciseID || check.Revision != revision {
		return training.Progress{}, fault.New(fault.Conflict, "CHAT_CHECK_REQUIRED", "Сначала проверьте итоговый ответ для текущего упражнения.")
	}
	attempt := training.Attempt{
		Sequence: state.AnswerCount + 1, ExerciseID: exerciseID, Round: state.Current.Round,
		TargetIndex: state.Current.TargetIndex, Text: check.FinalAnswer, Score: check.Score,
		MaxScore: 2, Verdict: check.Verdict, CriterionResults: append([]training.CriterionResult(nil), check.CriterionResults...),
		Feedback: append([]string(nil), check.Feedback...), CreatedAt: s.now().Unix(), AnswerSource: check.Source,
	}
	if check.TranscriptionID != nil {
		attempt.TranscriptionID = *check.TranscriptionID
	}
	if err = s.advance(ctx, &state, attempt); err != nil {
		return training.Progress{}, err
	}
	result, err := s.checkedNext.AcceptNext(ctx, *reservation, state, attempt)
	if err != nil {
		return training.Progress{}, err
	}
	failed = false
	return result, nil
}

func (s *Service) answerRequest(ctx context.Context, owner, id, exerciseID, key string, data []byte, media string, skip bool) (training.Progress, error) {
	if err := ValidKey(key); err != nil {
		return training.Progress{}, err
	}
	if !skip {
		if err := audio.Check(data, media); err != nil {
			return training.Progress{}, err
		}
	}
	if skip {
		media = "skip"
	}
	h := sha256.New()
	h.Write([]byte(exerciseID + "\x00" + media + "\x00"))
	h.Write(data)
	fingerprint := hex.EncodeToString(h.Sum(nil))
	token, err := s.ids.New("attempt")
	if err != nil {
		return training.Progress{}, err
	}
	replay, reserved, state, err := s.repo.Reserve(ctx, owner, id, key, fingerprint, exerciseID, token)
	if err != nil {
		return training.Progress{}, err
	}
	if replay != nil {
		return *replay, nil
	}
	defer func() { _ = s.repo.Fail(context.WithoutCancel(ctx), owner, id, *reserved) }()
	if state.Current.Stage == training.StageTheory {
		return training.Progress{}, theoryStage()
	}
	if skip {
		reserved.TranscriptionID, err = s.ids.New("skip")
		if err != nil {
			return training.Progress{}, err
		}
		reserved.Text = "Я не знаю. Пропустить"
	} else if reserved.TranscriptionID == "" {
		tr, err := s.voice.Transcribe(ctx, owner, data, media)
		if err != nil {
			return training.Progress{}, err
		}
		if tr.OwnerID != owner || tr.ID == "" || strings.TrimSpace(tr.Text) == "" {
			return training.Progress{}, fault.New(fault.Upstream, "TRANSCRIPTION_INVALID_RESPONSE", "Некорректная расшифровка.")
		}
		reserved.TranscriptionID = tr.ID
		reserved.Text = tr.Text
	}
	evaluation := assessment.Evaluation{}
	if skip {
		evaluation = assessment.Evaluation{Score: 0, MaxScore: 2, Verdict: "incorrect", Feedback: []string{"Вопрос пропущен.", "Ответ оценён в 0 баллов.", "Продолжите со следующим вопросом."}}
	} else {
		evaluation, err = s.grader.EvaluateTraining(ctx, owner, reserved.TranscriptionID, state.Current)
		if err != nil {
			return training.Progress{}, err
		}
		if evaluation.MaxScore != 2 || evaluation.Score < 0 || evaluation.Score > 2 {
			return training.Progress{}, fault.New(fault.Upstream, "INVALID_MODEL_RESPONSE", "Некорректная оценка тренировки.")
		}
	}
	attempt := training.Attempt{Sequence: state.AnswerCount + 1, ExerciseID: state.Current.ID, Round: state.Round, TargetIndex: state.Current.TargetIndex, TranscriptionID: reserved.TranscriptionID, Text: reserved.Text, Score: evaluation.Score, MaxScore: 2, Verdict: evaluation.Verdict, Feedback: evaluation.Feedback, CreatedAt: s.now().Unix(), CriterionResults: []training.CriterionResult{}, Skipped: skip}
	for _, c := range evaluation.CriterionResults {
		attempt.CriterionResults = append(attempt.CriterionResults, training.CriterionResult{Key: c.Key, Satisfied: c.Satisfied, Explanation: c.Explanation})
	}
	if err = s.advance(ctx, &state, attempt); err != nil {
		return training.Progress{}, err
	}
	return s.repo.Accept(ctx, owner, *reserved, state, attempt)
}
func (s *Service) History(ctx context.Context, owner, id string, before int64, limit int) ([]training.Attempt, error) {
	return s.repo.History(ctx, owner, id, before, limit)
}

func (s *Service) ResetReservation(ctx context.Context, owner, id string) error {
	return s.repo.ResetReservation(ctx, owner, id)
}
func (s *Service) CurrentAudio(ctx context.Context, owner, id, exerciseID string) (audioasset.Metadata, error) {
	state, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return audioasset.Metadata{}, err
	}
	if state.Current.ID != exerciseID {
		return audioasset.Metadata{}, fault.New(fault.Conflict, "TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	m := audioasset.Metadata{Status: audioasset.Missing}
	if state.Current.Task.AudioAssetID == nil {
		return m, nil
	}
	if s.audio == nil {
		return m, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Озвучка временно недоступна.")
	}
	asset, err := s.audio.Metadata(ctx, *state.Current.Task.AudioAssetID)
	if errors.Is(err, audioasset.ErrNotFound) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	m.Status = asset.Status
	if asset.Status == audioasset.Ready {
		if asset.AudioURL == nil || *asset.AudioURL != "/task-audio/"+asset.ID+"/file" {
			return m, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Озвучка временно недоступна.")
		}
		m.AudioURL = asset.AudioURL
	}
	return m, nil
}

func (s *Service) RegenerateCurrentAudio(ctx context.Context, owner, id, exerciseID string) (audioasset.Metadata, error) {
	state, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return audioasset.Metadata{}, err
	}
	if state.Current.ID != exerciseID {
		return audioasset.Metadata{}, fault.New(fault.Conflict, "TRAINING_EXERCISE_NOT_CURRENT", "Упражнение изменилось.")
	}
	if state.Current.Task.AudioAssetID == nil || *state.Current.Task.AudioAssetID == "" || s.regenerator == nil {
		return audioasset.Metadata{}, fault.New(fault.Conflict, "AUDIO_NOT_REPAIRABLE", "Озвучку сейчас нельзя восстановить.")
	}
	asset, err := s.regenerator.Regenerate(ctx, *state.Current.Task.AudioAssetID)
	if err != nil {
		return audioasset.Metadata{}, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Озвучка временно недоступна.")
	}
	if asset.Status != audioasset.Ready || asset.AudioURL == nil {
		return audioasset.Metadata{}, fault.New(fault.Unavailable, "AUDIO_STORAGE_UNAVAILABLE", "Озвучка временно недоступна.")
	}
	return audioasset.Metadata{Status: asset.Status, AudioURL: asset.AudioURL}, nil
}
