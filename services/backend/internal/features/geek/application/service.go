package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

const (
	closingText = "Спасибо! Диагностика завершена, итог уже готов."
	// speechLimit is the TTS limit of one synthesis request.
	speechLimit = 500
	maxKeyLen   = 200
	maxTextLen  = 4000
)

type Config struct {
	// MaxQuestions stops a diagnostic after that many questions; 0 means no limit.
	MaxQuestions int
}

type Service struct {
	graphs   GraphSource
	store    Store
	texts    Transcriptions
	examiner Examiner
	voice    Synthesizer
	audio    AudioStore
	ids      IDGenerator
	now      func() time.Time
	cfg      Config
}

func New(graphs GraphSource, store Store, texts Transcriptions, examiner Examiner, voice Synthesizer, audio AudioStore, ids IDGenerator, now func() time.Time, cfg Config) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{graphs: graphs, store: store, texts: texts, examiner: examiner, voice: voice, audio: audio, ids: ids, now: now, cfg: cfg}
}

// Answer is the student's reply: exactly one of a confirmed transcription, a
// text or the "don't know" button.
type Answer struct {
	TurnID          string
	TranscriptionID string
	Text            string
	DontKnow        bool
}

var errSessionNotFound = fault.New(fault.NotFound, "SESSION_NOT_FOUND", "Диагностика не найдена.")

func validKey(key string) error {
	if strings.TrimSpace(key) == "" || len(key) > maxKeyLen {
		return fault.Validation("Idempotency-Key", "Нужен ключ повтора запроса до 200 символов.")
	}
	return nil
}

// Start returns the student's active diagnostic on the course or starts a new
// one with its first question; created reports a new session.
func (s *Service) Start(ctx context.Context, ownerID, subjectID, key string) (exam.Session, bool, error) {
	if err := validKey(key); err != nil {
		return exam.Session{}, false, err
	}
	if strings.TrimSpace(subjectID) == "" {
		return exam.Session{}, false, fault.Validation("subject_id", "Укажите курс.")
	}
	fingerprint := "start:" + subjectID
	if r, ok, err := s.replay(ctx, ownerID, key, fingerprint); err != nil || ok {
		return r.Response, r.Created, err
	}
	if exists, err := s.store.SubjectExists(ctx, subjectID); err != nil {
		return exam.Session{}, false, err
	} else if !exists {
		return exam.Session{}, false, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Курс не найден.")
	}
	if active, ok, err := s.store.ActiveSession(ctx, ownerID, subjectID); err != nil {
		return exam.Session{}, false, err
	} else if ok {
		if active, err = s.refresh(ctx, active); err != nil {
			return exam.Session{}, false, err
		}
		if active.Status == exam.SessionActive {
			return s.resume(ctx, ownerID, key, fingerprint, active)
		}
	}
	graph, ok, err := s.graphs.Current(ctx, subjectID)
	if err != nil {
		return exam.Session{}, false, err
	}
	if !ok {
		return exam.Session{}, false, fault.New(fault.Conflict, "GRAPH_NOT_READY", "Граф курса ещё не собран.")
	}
	id, err := s.ids.New("diagnostic")
	if err != nil {
		return exam.Session{}, false, err
	}
	ex, step := exam.NewExam(graph, s.cfg.MaxQuestions)
	session := exam.Session{ID: id, OwnerID: ownerID, SubjectID: subjectID, Status: exam.SessionActive, StartedAt: s.now().Unix(), Version: 1, Exam: ex, Turns: []exam.Turn{}}
	if err := s.say(ctx, &session, step); err != nil {
		return exam.Session{}, false, err
	}
	request := Request{OwnerID: ownerID, Key: key, Fingerprint: fingerprint, SessionID: id, Created: true, Response: session, CreatedAt: s.now().Unix()}
	switch err := s.store.Create(ctx, session, request); {
	case errors.Is(err, ErrActiveExists):
		// A concurrent start won; resume its session.
		active, ok, err := s.store.ActiveSession(ctx, ownerID, subjectID)
		if err != nil || !ok {
			return exam.Session{}, false, errors.Join(ErrConflict, err)
		}
		return s.resume(ctx, ownerID, key, fingerprint, active)
	case err != nil:
		return exam.Session{}, false, err
	}
	return session, true, nil
}

// resume records the start request that returned an existing session, so its
// key keeps returning that result, and returns what is stored under the key.
func (s *Service) resume(ctx context.Context, ownerID, key, fingerprint string, active exam.Session) (exam.Session, bool, error) {
	request := Request{OwnerID: ownerID, Key: key, Fingerprint: fingerprint, SessionID: active.ID, Response: active, CreatedAt: s.now().Unix()}
	if err := s.store.SaveRequest(ctx, request); err != nil {
		return exam.Session{}, false, err
	}
	r, _, err := s.replay(ctx, ownerID, key, fingerprint)
	return r.Response, r.Created, err
}

// Get reads a diagnostic of its owner.
func (s *Service) Get(ctx context.Context, ownerID, id string) (exam.Session, error) {
	session, err := s.load(ctx, ownerID, id)
	if err != nil {
		return exam.Session{}, err
	}
	return s.refresh(ctx, session)
}

// History lists the student's diagnostics of a course, newest first; an active
// one over a rebuilt graph is cancelled on the way.
func (s *Service) History(ctx context.Context, ownerID, subjectID string) ([]exam.Session, error) {
	if strings.TrimSpace(subjectID) == "" {
		return nil, fault.Validation("subject_id", "Укажите курс.")
	}
	sessions, err := s.store.Sessions(ctx, ownerID, subjectID)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if sessions[i], err = s.refresh(ctx, sessions[i]); err != nil {
			return nil, err
		}
	}
	return sessions, nil
}

// Answer accepts the reply to the current Geek turn and returns the session
// with the next turn. The model is called before anything is saved, so a
// failure leaves the session unchanged and the request can be retried.
func (s *Service) Answer(ctx context.Context, ownerID, id, key string, a Answer) (exam.Session, error) {
	if err := validKey(key); err != nil {
		return exam.Session{}, err
	}
	fingerprint, err := answerFingerprint(id, a)
	if err != nil {
		return exam.Session{}, err
	}
	if r, ok, err := s.replay(ctx, ownerID, key, fingerprint); err != nil || ok {
		return r.Response, err
	}
	session, err := s.load(ctx, ownerID, id)
	if err != nil {
		return exam.Session{}, err
	}
	if session, err = s.refresh(ctx, session); err != nil {
		return exam.Session{}, err
	}
	current, ok := session.CurrentTurn()
	if !ok {
		return exam.Session{}, fault.New(fault.Conflict, "SESSION_NOT_ACTIVE", "Диагностика уже завершена.")
	}
	if current.ID != a.TurnID {
		return exam.Session{}, fault.New(fault.Conflict, "TURN_NOT_CURRENT", "Ответ относится не к текущему вопросу.")
	}
	text, source, err := s.answerText(ctx, ownerID, a)
	if err != nil {
		return exam.Session{}, err
	}
	previous := session.Version
	step := *session.Exam.Current
	node, _ := session.Exam.Graph.Node(step.NodeID)
	if err := s.append(&session, exam.SpeakerStudent, exam.KindAnswer, &step.NodeID, &source, text); err != nil {
		return exam.Session{}, err
	}
	evaluation := exam.Evaluation{Verdict: exam.VerdictDontKnow}
	if source != exam.SourceDontKnow {
		if evaluation, err = s.examiner.Evaluate(ctx, EvaluationRequest{Kind: step.Kind, Node: node, Dialogue: session.Dialogue(node.ID)}); err != nil {
			return exam.Session{}, modelError(err)
		}
		if err := checkEvaluation(evaluation, session); err != nil {
			return exam.Session{}, modelError(err)
		}
	}
	next := session.Exam.Apply(evaluation)
	if err := s.say(ctx, &session, next); err != nil {
		return exam.Session{}, err
	}
	session.Version++
	request := Request{OwnerID: ownerID, Key: key, Fingerprint: fingerprint, SessionID: id, Response: session, CreatedAt: s.now().Unix()}
	if err := s.store.Update(ctx, session, previous, &request); errors.Is(err, ErrConflict) {
		// A retry of the same request may have been accepted meanwhile.
		if r, ok, err := s.replay(ctx, ownerID, key, fingerprint); err != nil || ok {
			return r.Response, err
		}
		return exam.Session{}, fault.New(fault.Conflict, "TURN_NOT_CURRENT", "Диагностика изменилась; обновите её.")
	} else if err != nil {
		return exam.Session{}, err
	}
	return session, nil
}

// Result is the outcome of a completed diagnostic.
func (s *Service) Result(ctx context.Context, ownerID, id string) (exam.Result, error) {
	session, err := s.Get(ctx, ownerID, id)
	if err != nil {
		return exam.Result{}, err
	}
	if session.Status != exam.SessionCompleted {
		return exam.Result{}, fault.New(fault.Conflict, "SESSION_NOT_COMPLETED", "Диагностика ещё не завершена.")
	}
	return session.Result(), nil
}

// TurnAudio voices a Geek turn. The WAV is synthesized once, by parts within
// the TTS limit, and cached.
func (s *Service) TurnAudio(ctx context.Context, ownerID, id, turnID string) ([]byte, error) {
	session, err := s.load(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	turn, ok := session.Turn(turnID)
	if !ok || turn.Speaker != exam.SpeakerGeek {
		return nil, fault.New(fault.NotFound, "TURN_NOT_FOUND", "Реплика Geek не найдена.")
	}
	key := "geek/" + session.ID + "/" + turn.ID + ".wav"
	if data, ok, err := s.audio.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return data, nil
	}
	parts := [][]byte{}
	for _, chunk := range SplitSpeech(turn.Text, speechLimit) {
		data, err := s.voice.Synthesize(ctx, chunk)
		if err != nil {
			return nil, err
		}
		parts = append(parts, data)
	}
	data, ok := audio.JoinWAV(parts)
	if !ok {
		return nil, fault.New(fault.Upstream, "SYNTHESIS_FAILED", "Сервис синтеза вернул несовместимые части.")
	}
	if err := s.audio.Put(ctx, key, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) load(ctx context.Context, ownerID, id string) (exam.Session, error) {
	session, err := s.store.Session(ctx, ownerID, id)
	if errors.Is(err, ErrNotFound) {
		return exam.Session{}, errSessionNotFound
	}
	return session, err
}

// replay returns the stored result of a repeated request.
func (s *Service) replay(ctx context.Context, ownerID, key, fingerprint string) (Request, bool, error) {
	r, ok, err := s.store.Request(ctx, ownerID, key)
	if err != nil || !ok {
		return Request{}, false, err
	}
	if r.Fingerprint != fingerprint {
		return Request{}, true, fault.New(fault.Invalid, "IDEMPOTENCY_KEY_REUSED", "Ключ повтора уже использован для другого запроса.")
	}
	return r, true, nil
}

// refresh cancels an active diagnostic whose course graph was rebuilt.
func (s *Service) refresh(ctx context.Context, session exam.Session) (exam.Session, error) {
	if session.Status != exam.SessionActive {
		return session, nil
	}
	graph, ok, err := s.graphs.Current(ctx, session.SubjectID)
	if err != nil || !ok || graph.Version == session.Exam.Graph.Version {
		return session, err
	}
	previous := session.Version
	session.Status = exam.SessionCancelled
	session.Version++
	if err := s.store.Update(ctx, session, previous, nil); err != nil && !errors.Is(err, ErrConflict) {
		return exam.Session{}, err
	}
	return s.store.Session(ctx, session.OwnerID, session.ID)
}

// say appends the Geek turn for a step; the closing completes the session.
func (s *Service) say(ctx context.Context, session *exam.Session, step exam.Step) error {
	if step.Kind == exam.KindClosing {
		now := s.now().Unix()
		session.Status, session.CompletedAt = exam.SessionCompleted, &now
		return s.append(session, exam.SpeakerGeek, exam.KindClosing, nil, nil, closingText)
	}
	node, _ := session.Exam.Graph.Node(step.NodeID)
	var text string
	if step.Kind == exam.KindQuestion && len(node.Questions) > 0 && strings.TrimSpace(node.Questions[0]) != "" {
		text = node.Questions[0]
	} else {
		var err error
		text, err = s.examiner.Question(ctx, QuestionRequest{Kind: step.Kind, Node: node, Dialogue: session.Dialogue(node.ID)})
		if err != nil {
			return modelError(err)
		}
		if text = strings.TrimSpace(text); text == "" {
			return modelError(errors.New("empty question"))
		}
	}
	nodeID := step.NodeID
	return s.append(session, exam.SpeakerGeek, step.Kind, &nodeID, nil, text)
}

func (s *Service) append(session *exam.Session, speaker exam.Speaker, kind exam.TurnKind, nodeID *string, source *exam.Source, text string) error {
	id, err := s.ids.New("turn")
	if err != nil {
		return err
	}
	session.Turns = append(session.Turns, exam.Turn{ID: id, Line: len(session.Turns) + 1, Speaker: speaker, Kind: kind, NodeID: nodeID, Source: source, Text: text, CreatedAt: s.now().Unix()})
	return nil
}

func (s *Service) answerText(ctx context.Context, ownerID string, a Answer) (string, exam.Source, error) {
	switch {
	case a.DontKnow:
		return "Не знаю.", exam.SourceDontKnow, nil
	case a.TranscriptionID != "":
		text, ok, err := s.texts.Text(ctx, ownerID, a.TranscriptionID)
		if err != nil {
			return "", "", err
		}
		if !ok {
			return "", "", fault.New(fault.NotFound, "TRANSCRIPTION_NOT_FOUND", "Расшифровка не найдена.")
		}
		return text, exam.SourceVoice, nil
	}
	return strings.TrimSpace(a.Text), exam.SourceText, nil
}

func answerFingerprint(sessionID string, a Answer) (string, error) {
	set := 0
	if a.TranscriptionID != "" {
		set++
	}
	if a.Text != "" {
		set++
	}
	if a.DontKnow {
		set++
	}
	switch {
	case strings.TrimSpace(a.TurnID) == "":
		return "", fault.Validation("turn_id", "Укажите текущую реплику.")
	case set != 1:
		return "", fault.Validation("body", "Нужно ровно одно из transcription_id, text или dont_know.")
	case a.Text != "" && (strings.TrimSpace(a.Text) == "" || utf8.RuneCountInString(a.Text) > maxTextLen || strings.IndexByte(a.Text, 0) >= 0):
		return "", fault.Validation("text", "Ответ должен содержать 1–4000 символов.")
	}
	return fmt.Sprintf("answer:%s:%s:%s:%s:%t", sessionID, a.TurnID, a.TranscriptionID, a.Text, a.DontKnow), nil
}

func checkEvaluation(e exam.Evaluation, session exam.Session) error {
	switch e.Verdict {
	case exam.VerdictAnswered, exam.VerdictDontKnow, exam.VerdictInsufficient:
	default:
		return fmt.Errorf("unknown verdict %q", e.Verdict)
	}
	if !e.Criteria.Valid() {
		return fmt.Errorf("criteria out of range: %+v", e.Criteria)
	}
	for _, line := range e.Evidence {
		if line < 1 || line > len(session.Turns) {
			return fmt.Errorf("evidence line %d out of range", line)
		}
	}
	return nil
}

func modelError(err error) error {
	var f *fault.Error
	if errors.As(err, &f) {
		return err
	}
	return errors.Join(fault.New(fault.Unavailable, "MODEL_UNAVAILABLE", "Модель временно недоступна; повторите ответ."), err)
}
