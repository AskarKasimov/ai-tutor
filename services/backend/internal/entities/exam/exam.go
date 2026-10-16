package exam

import "slices"

type Status string

const (
	StatusUnchecked    Status = "unchecked"
	StatusIndependent  Status = "independent"
	StatusAssisted     Status = "assisted"
	StatusGap          Status = "gap"
	StatusInsufficient Status = "insufficient"
	StatusCredited     Status = "credited_by_parent"
)

// Mastered reports whether a status counts the topic as learned.
func (s Status) Mastered() bool {
	return s == StatusIndependent || s == StatusAssisted || s == StatusCredited
}

type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionCancelled SessionStatus = "cancelled"
)

type Speaker string

const (
	SpeakerGeek    Speaker = "geek"
	SpeakerStudent Speaker = "student"
)

type TurnKind string

const (
	KindQuestion TurnKind = "question"
	KindFollowup TurnKind = "followup"
	KindRephrase TurnKind = "rephrase"
	KindClosing  TurnKind = "closing"
	KindAnswer   TurnKind = "answer"
)

type Source string

const (
	SourceVoice    Source = "voice"
	SourceText     Source = "text"
	SourceDontKnow Source = "dont_know"
)

// Turn is one line of the transcript.
type Turn struct {
	ID        string   `json:"id"`
	Line      int      `json:"line"`
	Speaker   Speaker  `json:"speaker"`
	Kind      TurnKind `json:"kind"`
	NodeID    *string  `json:"node_id"`
	Source    *Source  `json:"source"`
	Text      string   `json:"text"`
	CreatedAt int64    `json:"created_at"`
}

// NodeState is what the exam has concluded about one topic. Criteria and
// evidence lines are kept for the teacher package.
type NodeState struct {
	Status   Status    `json:"status"`
	Grade    *Grade    `json:"grade"`
	Criteria *Criteria `json:"criteria"`
	Evidence []int     `json:"evidence"`
	Assisted bool      `json:"assisted"`
	Asked    bool      `json:"asked"`
}

// Verdict is the model's reading of a student's answer.
type Verdict string

const (
	VerdictAnswered     Verdict = "answered"
	VerdictDontKnow     Verdict = "dont_know"
	VerdictInsufficient Verdict = "insufficient"
)

// Evaluation is the model's assessment of the dialogue on the current topic.
type Evaluation struct {
	Verdict  Verdict
	Criteria Criteria
	Evidence []int
}

// Step is what Geek asks next.
type Step struct {
	Kind   TurnKind // question, followup, rephrase or closing
	NodeID string   // empty for closing
}

// Exam is the traversal state of one diagnostic, stored with the session.
type Exam struct {
	Graph   Graph                 `json:"graph"`
	Nodes   map[string]*NodeState `json:"nodes"`
	Stack   []string              `json:"stack"`
	Top     string                `json:"top"`
	Done    []string              `json:"done"`
	Current *Step                 `json:"current"`
	// Asked counts questions and followups; rephrases do not count.
	Asked int `json:"asked"`
	// MaxQuestions stops the exam once reached; 0 means no limit.
	MaxQuestions int `json:"max_questions"`
}

// NewExam starts the traversal from the top-level topics in study order and
// returns the first step.
func NewExam(g Graph, maxQuestions int) (*Exam, Step) {
	e := &Exam{Graph: g, Nodes: map[string]*NodeState{}, MaxQuestions: maxQuestions}
	for _, n := range g.Nodes {
		e.Nodes[n.ID] = &NodeState{Status: StatusUnchecked, Evidence: []int{}}
	}
	top := g.TopLevel()
	for i := len(top) - 1; i >= 0; i-- {
		e.Stack = append(e.Stack, top[i])
	}
	return e, e.next()
}

func (e *Exam) isTop(id string) bool { return slices.Contains(e.Graph.TopLevel(), id) }

// next pops the next unchecked topic depth-first. Popping a new top-level topic
// finishes the previous one; an empty stack completes the exam.
func (e *Exam) next() Step {
	for len(e.Stack) > 0 {
		if e.MaxQuestions > 0 && e.Asked >= e.MaxQuestions {
			break
		}
		id := e.Stack[len(e.Stack)-1]
		e.Stack = e.Stack[:len(e.Stack)-1]
		if e.Nodes[id].Status != StatusUnchecked {
			continue
		}
		if e.isTop(id) {
			e.finishTop()
			e.Top = id
		}
		e.Asked++
		step := Step{Kind: KindQuestion, NodeID: id}
		e.Current = &step
		return step
	}
	e.finishTop()
	e.Stack = nil
	step := Step{Kind: KindClosing}
	e.Current = &step
	return step
}

func (e *Exam) finishTop() {
	if e.Top != "" && !slices.Contains(e.Done, e.Top) {
		e.Done = append(e.Done, e.Top)
	}
	e.Top = ""
}

// Apply records the evaluation of the answer to the current step and returns
// the next step. A "don't know" on the main question or the rephrase makes the
// topic a gap at once. A main answer on D is rephrased once; an answer after
// the rephrase is assisted. A non-D answer gets one followup on K3 and K4, and
// the grade is set after it; "don't know" or an unclear answer on the followup
// sets K3 and K4 to 0 and grades by the table. A gap leads to its prerequisites; a mastered
// topic credits all of its unchecked prerequisites.
func (e *Exam) Apply(ev Evaluation) Step {
	cur := *e.Current
	state := e.Nodes[cur.NodeID]
	state.Asked = true
	state.Evidence = mergeLines(state.Evidence, ev.Evidence)
	switch cur.Kind {
	case KindQuestion, KindRephrase:
		assisted := cur.Kind == KindRephrase
		switch ev.Verdict {
		case VerdictDontKnow:
			return e.gap(cur.NodeID, Criteria{}, assisted)
		case VerdictInsufficient:
			if !assisted {
				return e.step(KindRephrase, cur.NodeID)
			}
			state.Status = StatusInsufficient
			state.Assisted = true
			return e.next()
		}
		c := ev.Criteria
		state.Criteria = &c
		state.Assisted = assisted
		if TopicGrade(c, assisted) == GradeD {
			if !assisted {
				return e.step(KindRephrase, cur.NodeID)
			}
			return e.gap(cur.NodeID, c, true)
		}
		e.Asked++
		return e.step(KindFollowup, cur.NodeID)
	case KindFollowup:
		c := ev.Criteria
		if ev.Verdict != VerdictAnswered {
			c = *state.Criteria
			c.K3, c.K4 = 0, 0
		}
		state.Criteria = &c
		grade := TopicGrade(c, state.Assisted)
		if grade == GradeD {
			return e.gap(cur.NodeID, c, state.Assisted)
		}
		state.Grade = &grade
		state.Status = StatusIndependent
		if state.Assisted {
			state.Status = StatusAssisted
		}
		for _, p := range e.Graph.Closure(cur.NodeID) {
			if e.Nodes[p].Status == StatusUnchecked {
				e.Nodes[p].Status = StatusCredited
			}
		}
		return e.next()
	}
	return e.next()
}

func (e *Exam) step(kind TurnKind, id string) Step {
	s := Step{Kind: kind, NodeID: id}
	e.Current = &s
	return s
}

// gap marks a topic as a gap and descends into its prerequisites first.
func (e *Exam) gap(id string, c Criteria, assisted bool) Step {
	state := e.Nodes[id]
	d := GradeD
	state.Status, state.Grade, state.Criteria, state.Assisted = StatusGap, &d, &c, assisted
	n, _ := e.Graph.Node(id)
	for i := len(n.Prerequisites) - 1; i >= 0; i-- {
		e.Stack = append(e.Stack, n.Prerequisites[i])
	}
	return e.next()
}

func mergeLines(a, b []int) []int {
	out := slices.Clone(a)
	for _, x := range b {
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	slices.Sort(out)
	return out
}

// Completed reports whether the exam has reached its closing.
func (e *Exam) Completed() bool { return e.Current != nil && e.Current.Kind == KindClosing }

// Progress is the share of top-level topics concluded, in percent. Each
// top-level topic has an equal share; while it is in progress its share grows
// with the concluded topics among it and its prerequisites, but never fills.
func (e *Exam) Progress() int {
	top := e.Graph.TopLevel()
	if len(top) == 0 || e.Completed() {
		return 100
	}
	var sum float64
	for _, t := range top {
		if slices.Contains(e.Done, t) {
			sum++
			continue
		}
		scope := append([]string{t}, e.Graph.Closure(t)...)
		concluded := 0
		for _, id := range scope {
			if e.Nodes[id].Status != StatusUnchecked {
				concluded++
			}
		}
		share := float64(concluded) / float64(len(scope))
		sum += min(share, 0.99)
	}
	return min(99, int(sum/float64(len(top))*100))
}

// Gaps are the deepest unmastered topics: gaps all of whose prerequisites are
// mastered. A gap over an unchecked or insufficient prerequisite is not listed,
// since whether its prerequisites are learned is unknown.
func (e *Exam) Gaps() []string {
	out := []string{}
	for _, n := range e.Graph.Nodes {
		if e.Nodes[n.ID].Status != StatusGap {
			continue
		}
		deepest := true
		for _, p := range n.Prerequisites {
			if !e.Nodes[p].Status.Mastered() {
				deepest = false
			}
		}
		if deepest {
			out = append(out, n.ID)
		}
	}
	return out
}

// CourseGrade grades the course by its top-level topics.
func (e *Exam) CourseGrade() (*Grade, *int) {
	grades := []Grade{}
	for _, t := range e.Graph.TopLevel() {
		if g := e.Nodes[t].Grade; g != nil {
			grades = append(grades, *g)
		}
	}
	return CourseGrade(grades)
}
