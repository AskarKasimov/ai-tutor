// Package application runs the Geek oral exam: it starts diagnostics, accepts
// answers, asks the model for questions and evaluations and voices Geek turns.
package application

import (
	"context"
	"errors"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

var (
	ErrNotFound     = errors.New("geek session not found")
	ErrActiveExists = errors.New("active geek session already exists")
	ErrConflict     = errors.New("geek session changed concurrently")
)

// GraphSource returns the current graph of a course; ok is false when the
// course has no graph yet.
type GraphSource interface {
	Current(ctx context.Context, subjectID string) (g exam.Graph, ok bool, err error)
}

// Request is a stored idempotent request and the session it produced.
type Request struct {
	OwnerID     string
	Key         string
	Fingerprint string
	SessionID   string
	Created     bool
	Response    exam.Session
	CreatedAt   int64
}

type Store interface {
	SubjectExists(ctx context.Context, subjectID string) (bool, error)
	Session(ctx context.Context, ownerID, id string) (exam.Session, error)
	ActiveSession(ctx context.Context, ownerID, subjectID string) (exam.Session, bool, error)
	// Sessions lists the owner's diagnostics of a course, newest first.
	Sessions(ctx context.Context, ownerID, subjectID string) ([]exam.Session, error)
	Request(ctx context.Context, ownerID, key string) (Request, bool, error)
	// Create inserts a session; ErrActiveExists if the owner already has an
	// active one on the course.
	Create(ctx context.Context, s exam.Session, r Request) error
	// Update saves s if its stored version is still previous; otherwise
	// ErrConflict. A non-nil request is saved in the same transaction.
	Update(ctx context.Context, s exam.Session, previous int, r *Request) error
	// SaveRequest records a request that did not change a session; a request
	// already stored under the key is kept.
	SaveRequest(ctx context.Context, r Request) error
}

// Transcriptions reads a transcription owned by the student.
type Transcriptions interface {
	Text(ctx context.Context, ownerID, id string) (text string, ok bool, err error)
}

type QuestionRequest struct {
	Kind     exam.TurnKind // question, followup or rephrase
	Node     exam.Node
	Dialogue []exam.Turn
}

type EvaluationRequest struct {
	Kind exam.TurnKind // the step that was answered
	Node exam.Node
	// Prerequisites are the titles of the topic's prerequisites. Geek may ask
	// them next, so the explanation must not give them away.
	Prerequisites []string
	Dialogue      []exam.Turn // turns on the topic, ending with the answer
}

// Assessment is the evaluation of an answer with Geek's reply to it. The
// reaction says what is right or missing without the correct answer; the
// explanation adds the correct answer and is said once the topic is closed.
type Assessment struct {
	exam.Evaluation
	Reaction    string
	Explanation string
}

// Examiner is the language model behind Geek.
type Examiner interface {
	Question(ctx context.Context, r QuestionRequest) (string, error)
	Evaluate(ctx context.Context, r EvaluationRequest) (Assessment, error)
	// Explain gives the correct answer on a topic the student gave up on with
	// the "don't know" button, which is not evaluated.
	Explain(ctx context.Context, r EvaluationRequest) (string, error)
}

type Synthesizer interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

// AudioStore caches synthesized Geek turns.
type AudioStore interface {
	Get(ctx context.Context, key string) (data []byte, ok bool, err error)
	Put(ctx context.Context, key string, data []byte) error
}

type IDGenerator interface {
	New(prefix string) (string, error)
}
