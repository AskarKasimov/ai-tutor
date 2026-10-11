package training

import "time"

const TutorPromptVersion = "training-tutor-v1"

type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
)

type ChatKind string

const (
	ChatKindWelcome ChatKind = "welcome"
	ChatKindMessage ChatKind = "message"
	ChatKindCheck   ChatKind = "check"
)

type AnswerSource string

const (
	AnswerSourceText  AnswerSource = "text"
	AnswerSourceVoice AnswerSource = "voice"
)

type Pedagogy string

const (
	PedagogyQuestion      Pedagogy = "question"
	PedagogyHint          Pedagogy = "hint"
	PedagogyConcept       Pedagogy = "concept"
	PedagogyMisconception Pedagogy = "misconception"
	PedagogyConfirmation  Pedagogy = "confirmation"
	PedagogyClarification Pedagogy = "clarification"
)

type ChatMessage struct {
	ID        string       `json:"id"`
	Sequence  int64        `json:"sequence"`
	Role      ChatRole     `json:"role"`
	Kind      ChatKind     `json:"kind"`
	Text      string       `json:"text"`
	Source    AnswerSource `json:"source,omitempty" binding:"optional"`
	CreatedAt int64        `json:"created_at"`
}

type Check struct {
	Revision         int64
	FinalAnswer      string
	Source           AnswerSource
	TranscriptionID  *string
	Score            int
	MaxScore         int
	Verdict          string
	CriterionResults []CriterionResult
	Feedback         []string
}

func (c Check) IsCurrent(revision int64) bool { return c.Revision == revision }

type TutorReply struct {
	AssistantText string
	Pedagogy      Pedagogy
}

type TutorContext struct {
	Exercise    Exercise
	Summary     string
	Recent      []ChatMessage
	StudentText string
	Kind        ChatKind
	Check       *Check
}

type TutorInput struct {
	Text    string
	Kind    ChatKind
	Check   *Check
	Summary string
}

type ContextLimits struct {
	TaskMaxChars    int
	HistoryMaxChars int
	SummaryMaxChars int
}

type ChatStatus string

const (
	ChatStatusActive ChatStatus = "active"
	ChatStatusClosed ChatStatus = "closed"
)

type TurnStatus string

const (
	TurnStatusProcessing TurnStatus = "processing"
	TurnStatusFailed     TurnStatus = "failed"
	TurnStatusCompleted  TurnStatus = "completed"
	TurnStatusCancelled  TurnStatus = "cancelled"
)

type Turn struct {
	ID               string
	SessionID        string
	ExerciseID       string
	RequestKey       string
	Operation        ChatKind
	Fingerprint      string
	ExpectedRevision int64
	Token            string
	LeaseUntil       time.Time
	Status           TurnStatus
	Source           AnswerSource
	Text             string
	TranscriptionID  *string
	PendingAudio     []byte
	ErrorCode        string
	Result           *ChatTurnResult
}

type ChatTurnResult struct {
	SessionID   string         `json:"session_id"`
	ExerciseID  string         `json:"exercise_id"`
	Revision    int64          `json:"revision"`
	TurnID      string         `json:"turn_id"`
	Messages    [2]ChatMessage `json:"messages"`
	LatestCheck *ChatCheckView `json:"latest_check"`
}

type ChatCheckView struct {
	Revision        int64        `json:"revision"`
	FinalAnswer     string       `json:"final_answer"`
	Source          AnswerSource `json:"source"`
	TranscriptionID *string      `json:"transcription_id,omitempty" binding:"optional"`
	Score           int          `json:"score"`
	MaxScore        int          `json:"max_score"`
	Verdict         string       `json:"verdict"`
}

type PendingTurnView struct {
	ID         string       `json:"id"`
	Kind       ChatKind     `json:"kind"`
	Source     AnswerSource `json:"source"`
	Status     TurnStatus   `json:"status"`
	UserText   *string      `json:"user_text"`
	RetryAfter int64        `json:"retry_after_ms"`
}

type ChatPage struct {
	SessionID   string           `json:"session_id"`
	ExerciseID  string           `json:"exercise_id"`
	Revision    int64            `json:"revision"`
	Status      ChatStatus       `json:"status"`
	Items       []ChatMessage    `json:"items"`
	NextCursor  *string          `json:"next_cursor,omitempty" binding:"optional"`
	LatestCheck *ChatCheckView   `json:"latest_check"`
	PendingTurn *PendingTurnView `json:"pending_turn"`
}

type TurnInput struct {
	ExerciseID string
	Key        string
	Revision   int64
	Kind       ChatKind
	Source     AnswerSource
	Text       string
	Audio      []byte
	MediaType  string
}

type TurnReservation struct {
	OwnerID   string
	SessionID string
	Exercise  Exercise
	Turn      Turn
	History   []ChatMessage
	Input     TurnInput
	Summary   string
}

type TurnCommit struct {
	Messages      [2]ChatMessage
	Check         *Check
	PromptVersion string
}

type NextReservation struct {
	OwnerID     string
	SessionID   string
	ExerciseID  string
	Token       string
	Key         string
	Fingerprint string
	Session     Session
	Check       Check
}

type TurnResult struct {
	Page     ChatPage
	Response ChatTurnResult
}
