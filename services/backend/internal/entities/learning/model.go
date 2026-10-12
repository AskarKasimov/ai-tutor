// Package learning describes the personal learning plan built after a
// diagnostic: the outcomes of every competency with a mistake, taught one by
// one in a chat — theory first, then practice until the outcome is mastered.
package learning

// ItemStatus is the state of one outcome in the plan.
type ItemStatus string

const (
	ItemPending ItemStatus = "pending" // ahead in the plan
	ItemCurrent ItemStatus = "current" // being learned now
	ItemPassed  ItemStatus = "passed"  // practice answered correctly
	ItemReview  ItemStatus = "review"  // attempts used up, worth repeating
)

// Detail is how thoroughly theory is explained: the fewer hours the student
// has for the whole plan, the shorter each explanation.
type Detail string

const (
	DetailBrief    Detail = "brief"
	DetailStandard Detail = "standard"
	DetailDeep     Detail = "deep"
)

// MaxAttempts is how many practice answers an outcome gets before the plan
// moves on and marks it for review.
const MaxAttempts = 3

// Default planned minutes when the map leaves the cells empty.
const (
	DefaultTheoryMinutes   = 15
	DefaultPracticeMinutes = 15
)

// Item is one outcome of the plan. Items are grouped by competency in map
// order; InPlan is false for outcomes that did not fit into the hours.
type Item struct {
	CompetencyID       string     `json:"competency_id"`
	CompetencyName     string     `json:"competency_name"`
	OutcomeID          string     `json:"outcome_id"`
	OutcomeName        string     `json:"outcome_name"`
	TaxonomyCode       string     `json:"taxonomy_code,omitempty"`
	Importance         int        `json:"importance"`
	EducationalContent string     `json:"educational_content,omitempty"`
	TheoryMinutes      int        `json:"theory_minutes"`
	PracticeMinutes    int        `json:"practice_minutes"`
	InPlan             bool       `json:"in_plan"`
	Status             ItemStatus `json:"status"`
	Attempts           int        `json:"attempts"`
	// Question is the practice question waiting for an answer.
	Question string `json:"question,omitempty"`
}

// Minutes is the planned time of the item.
func (i Item) Minutes() int { return i.TheoryMinutes + i.PracticeMinutes }

// Plan is a student's learning plan for one completed diagnostic.
type Plan struct {
	ID           string `json:"id"`
	OwnerID      string `json:"owner_id"`
	DiagnosticID string `json:"diagnostic_id"`
	SubjectID    string `json:"subject_id"`
	SubjectName  string `json:"subject_name"`
	Hours        int    `json:"hours"`
	Detail       Detail `json:"detail"`
	Items        []Item `json:"items"`
	// Current is the index of the item being learned, -1 when the plan is done.
	Current int `json:"current"`
	// PendingAnswer is a transcribed answer that still waits for its check,
	// for example after the model failed; the check can be retried.
	PendingAnswer *string `json:"pending_answer,omitempty"`
	CreatedAt     int64   `json:"created_at"`
}

// Completed reports whether every item has been gone through.
func (p Plan) Completed() bool { return p.Current < 0 }

// Progress is the share of finished items, in whole percent.
func (p Plan) Progress() int {
	if len(p.Items) == 0 {
		return 100
	}
	done := 0
	for _, item := range p.Items {
		if item.Status == ItemPassed || item.Status == ItemReview {
			done++
		}
	}
	return done * 100 / len(p.Items)
}

// Next is the index of the item after the current one: the rest of the plan
// first, then outcomes that did not fit into the hours; -1 when none is left.
func (p Plan) Next() int {
	for _, inPlan := range []bool{true, false} {
		for i, item := range p.Items {
			if item.InPlan == inPlan && item.Status == ItemPending {
				return i
			}
		}
	}
	return -1
}

type MessageRole string

const (
	RoleAssistant MessageRole = "assistant"
	RoleUser      MessageRole = "user"
)

type MessageKind string

const (
	KindTheory   MessageKind = "theory"
	KindPractice MessageKind = "practice"
	KindAnswer   MessageKind = "answer"
	KindFeedback MessageKind = "feedback"
)

// Message is one line of the lesson chat of a plan item.
type Message struct {
	Sequence  int64       `json:"sequence"`
	ItemIndex int         `json:"item_index"`
	Role      MessageRole `json:"role"`
	Kind      MessageKind `json:"kind"`
	Text      string      `json:"text"`
	CreatedAt int64       `json:"created_at"`
}

// Outcome is a map outcome a plan can teach.
type Outcome struct {
	CompetencyID       string
	CompetencyName     string
	OutcomeID          string
	OutcomeName        string
	TaxonomyCode       string
	Importance         int
	EducationalContent string
	TheoryMinutes      *int
	PracticeMinutes    *int
}

// Lesson is what the model teaches when an item opens.
type Lesson struct {
	Theory   string
	Question string
}

// LessonInput describes the outcome to teach.
type LessonInput struct {
	SubjectName string
	Item        Item
	Detail      Detail
}

// CheckInput is a practice answer to evaluate.
type CheckInput struct {
	SubjectName string
	Item        Item
	Detail      Detail
	Question    string
	Answer      string
	// LastAttempt means no further practice follows a wrong answer.
	LastAttempt bool
}

// Check is the model's verdict on a practice answer. For a wrong answer
// Feedback explains the mistake from the student's own answer and, unless it
// was the last attempt, Question is the next practice question.
type Check struct {
	Correct  bool
	Feedback string
	Question string
}
