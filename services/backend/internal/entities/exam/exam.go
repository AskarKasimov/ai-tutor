package exam

import (
	"math/rand/v2"
	"slices"
)

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
	Meta       bool     `json:"meta,omitempty"`
	AudioIntro string   `json:"audio_intro,omitempty"`
	ID         string   `json:"id"`
	Line       int      `json:"line"`
	Speaker    Speaker  `json:"speaker"`
	Kind       TurnKind `json:"kind"`
	NodeID     *string  `json:"node_id"`
	Source     *Source  `json:"source"`
	Text       string   `json:"text"`
	CreatedAt  int64    `json:"created_at"`
	// Recap is the start of Text that explains the previous, closed topic
	// before this turn's question; prompts on this turn's topic leave it out.
	Recap string `json:"recap,omitempty"`
	// Reply is the start of Text that answers the student's previous reply,
	// before this turn's question; it is voiced on its own, so the short
	// reaction plays while the question is still being synthesized.
	Reply string `json:"reply,omitempty"`
}

// NodeState is what the exam has concluded about one topic. Criteria and
// evidence lines are kept for the teacher package.
type NodeState struct {
	// Rubric is authoritative when present; Criteria is then a legacy
	// compatibility field, not an additional set of failed skills.
	Rubric   *RubricResult `json:"rubric,omitempty"`
	HintUsed bool          `json:"hint_used,omitempty"`
	// nil is a legacy session; new model evaluations always state completeness.
	AnswerComplete *bool              `json:"answer_complete,omitempty"`
	Unassessed     UnassessedCriteria `json:"unassessed,omitempty"`
	Status         Status             `json:"status"`
	Grade          *Grade             `json:"grade"`
	Criteria       *Criteria          `json:"criteria"`
	Evidence       []int              `json:"evidence"`
	Assisted       bool               `json:"assisted"`
	Asked          bool               `json:"asked"`
}

// Verdict is the model's reading of a student's answer.
type Verdict string

const (
	VerdictAnswered          Verdict = "answered"
	VerdictDontKnow          Verdict = "dont_know"
	VerdictInsufficient      Verdict = "insufficient"
	VerdictHintRequested     Verdict = "hint_requested"
	VerdictAddressCorrection Verdict = "address_correction"
	VerdictClarifyQuestion   Verdict = "clarify_question"
	VerdictPause             Verdict = "pause"
)

func (v Verdict) Conversational() bool {
	return v == VerdictAddressCorrection || v == VerdictClarifyQuestion || v == VerdictPause
}

// Evaluation is the model's assessment of the dialogue on the current topic.
type Evaluation struct {
	Rubric         *RubricResult
	AnswerComplete *bool
	Unassessed     UnassessedCriteria
	Verdict        Verdict
	Criteria       Criteria
	Evidence       []int
}

// Step is what Geek asks next.
type Step struct {
	Hint   bool     `json:"hint,omitempty"` // explain the setup, then let the student answer
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
	// Asked counts questions, followups and rephrases (decision of 18.10.2026).
	Asked int `json:"asked"`
	// MaxQuestions stops the exam once reached; 0 means no limit.
	MaxQuestions int `json:"max_questions"`
}

// NewExam starts the traversal from the top-level topics in coverage order and
// returns the first step. rng breaks ties of equal coverage and picks the
// task of every topic with variants, which the exam then keeps; nil keeps
// study order and the first variant.
func NewExam(g Graph, maxQuestions int, rng *rand.Rand) (*Exam, Step) {
	g = g.pickVariants(rng)
	e := &Exam{Graph: g, Nodes: map[string]*NodeState{}, MaxQuestions: maxQuestions}
	for _, n := range g.Nodes {
		e.Nodes[n.ID] = &NodeState{Status: StatusUnchecked, Evidence: []int{}}
	}
	top := coverageOrder(g, rng)
	for i := len(top) - 1; i >= 0; i-- {
		e.Stack = append(e.Stack, top[i])
	}
	return e, e.next()
}

// coverageOrder orders the top-level topics greedily: next is the one whose
// topic and prerequisites add the most topics not covered by those before it,
// since a mastered topic credits all of its prerequisites. Under a question
// limit the exam thus checks as much of the graph as it can. Ties (such as
// topics without prerequisites) are shuffled by rng, so different students
// get different ones.
func coverageOrder(g Graph, rng *rand.Rand) []string {
	left := g.TopLevel()
	if rng != nil {
		rng.Shuffle(len(left), func(i, j int) { left[i], left[j] = left[j], left[i] })
	}
	// Each scope is computed once: recomputing closures inside the greedy loop
	// made the order cubic in the number of topics.
	index := g.prerequisites()
	scope := make(map[string][]string, len(left))
	for _, id := range left {
		scope[id] = append([]string{id}, closure(index, id)...)
	}
	covered := map[string]bool{}
	order := make([]string, 0, len(left))
	for len(left) > 0 {
		best, bestGain := 0, -1
		for i, id := range left {
			gain := 0
			for _, n := range scope[id] {
				if !covered[n] {
					gain++
				}
			}
			if gain > bestGain {
				best, bestGain = i, gain
			}
		}
		for _, n := range scope[left[best]] {
			covered[n] = true
		}
		order = append(order, left[best])
		left = slices.Delete(left, best, best+1)
	}
	return order
}

func (e *Exam) isTop(id string) bool { return slices.Contains(e.Graph.TopLevel(), id) }

// limitReached reports whether the question limit leaves no room for another
// question, followup or rephrase.
func (e *Exam) limitReached() bool { return e.MaxQuestions > 0 && e.Asked >= e.MaxQuestions }

// next pops the next unchecked topic depth-first. Popping a new top-level topic
// finishes the previous one; an empty stack completes the exam.
func (e *Exam) next() Step {
	for len(e.Stack) > 0 {
		if e.limitReached() {
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
// topic a gap at once. A main answer on D or with insufficient data is
// rephrased once; an answer after the rephrase is assisted. Without room in
// the question limit for the rephrase, the main answer decides: D is a gap,
// insufficient data stays insufficient. A non-D answer that fully demonstrates
// the assessed criteria, or leaves no room in the limit, is graded at once.
// Otherwise one followup clarifies the same concept; an unclear answer or
// "don't know" keeps the skills already observed. Untested K3/K4 are excluded
// from grading, not treated as failed. A gap leads to its prerequisites; a
// mastered topic credits all of its unchecked prerequisites.
func (e *Exam) Apply(ev Evaluation) Step {
	cur := *e.Current
	if ev.Verdict.Conversational() {
		return cur
	}
	state := e.Nodes[cur.NodeID]
	state.Asked = true
	state.Evidence = mergeLines(state.Evidence, ev.Evidence)
	if ev.Verdict == VerdictHintRequested {
		if !state.HintUsed && !e.limitReached() {
			state.HintUsed, state.Assisted = true, true
			e.Asked++
			next := e.step(KindRephrase, cur.NodeID)
			next.Hint = true
			e.Current = &next
			return next
		}
		// One hint per topic; repeated requests cannot keep the exam open.
		ev.Verdict = VerdictInsufficient
	}
	switch cur.Kind {
	case KindQuestion, KindRephrase:
		assisted := cur.Kind == KindRephrase || state.HintUsed
		switch ev.Verdict {
		case VerdictDontKnow:
			return e.gap(cur.NodeID, Criteria{}, assisted)
		case VerdictInsufficient:
			if !assisted && !e.limitReached() {
				e.Asked++
				return e.step(KindRephrase, cur.NodeID)
			}
			state.Status = StatusInsufficient
			state.Assisted = assisted
			return e.next()
		}
		c := ev.Criteria
		state.Criteria = &c
		state.Rubric = ev.Rubric
		state.AnswerComplete = ev.AnswerComplete
		state.Unassessed = ev.Unassessed
		state.Assisted = assisted
		if state.topicGrade() == GradeD {
			if !assisted && !e.limitReached() {
				e.Asked++
				return e.step(KindRephrase, cur.NodeID)
			}
			return e.gap(cur.NodeID, c, assisted)
		}
		// A good answer to the selected concept is enough; untested skills
		// do not force another question. The question limit remains strict.
		if state.skillsComplete(c) && state.complete() || e.limitReached() {
			return e.conclude(cur.NodeID, c)
		}
		e.Asked++
		return e.step(KindFollowup, cur.NodeID)
	case KindFollowup:
		c := ev.Criteria
		if ev.Verdict != VerdictAnswered {
			c = *state.Criteria
		} else {
			state.Rubric = ev.Rubric
			state.AnswerComplete = ev.AnswerComplete
			// A skill observed earlier cannot become untested on a followup.
			if ev.Unassessed.K3 && !state.Unassessed.K3 {
				c.K3 = state.Criteria.K3
			}
			if ev.Unassessed.K4 && !state.Unassessed.K4 {
				c.K4 = state.Criteria.K4
			}
			state.Unassessed = UnassessedCriteria{
				K3: state.Unassessed.K3 && ev.Unassessed.K3,
				K4: state.Unassessed.K4 && ev.Unassessed.K4,
			}
		}
		return e.conclude(cur.NodeID, c)
	}
	return e.next()
}

// conclude grades a topic by its final criteria: D makes it a gap, any other
// grade masters it. A and B credit its unchecked prerequisites; C, a partial
// answer, keeps the grade but checks its prerequisites as a gap does
// (decision of 19.10.2026, as main 0/1 led to the basic tasks in v0.0.2).
func (e *Exam) conclude(id string, c Criteria) Step {
	state := e.Nodes[id]
	state.Criteria = &c
	grade := state.topicGrade()
	if grade == GradeD {
		return e.gap(id, c, state.Assisted)
	}
	state.Grade = &grade
	state.Status = StatusIndependent
	if state.Assisted {
		state.Status = StatusAssisted
	}
	if grade == GradeC {
		e.descend(id)
		return e.next()
	}
	for _, p := range e.Graph.Closure(id) {
		if e.Nodes[p].Status == StatusUnchecked {
			e.Nodes[p].Status = StatusCredited
		}
	}
	return e.next()
}

func (s NodeState) complete() bool {
	return s.AnswerComplete == nil || *s.AnswerComplete
}

func (s NodeState) topicGrade() Grade {
	if s.Rubric != nil {
		return s.Rubric.Grade(s.Assisted, s.complete())
	}
	grade := TopicGrade(*s.Criteria, s.Assisted, s.Unassessed)
	if grade == GradeA && !s.complete() {
		return GradeB
	}
	return grade
}

func (s NodeState) skillsComplete(c Criteria) bool {
	if s.Rubric != nil {
		return s.Rubric.Complete()
	}
	return s.Unassessed.Complete(c)
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
	e.descend(id)
	return e.next()
}

// descend asks the prerequisites of a topic next, in study order.
func (e *Exam) descend(id string) {
	n, _ := e.Graph.Node(id)
	for i := len(n.Prerequisites) - 1; i >= 0; i-- {
		e.Stack = append(e.Stack, n.Prerequisites[i])
	}
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
// Under a question limit the exam may close before all top-level topics, so
// the share of the limit already asked counts too.
func (e *Exam) Progress() int {
	top := e.Graph.TopLevel()
	if len(top) == 0 || e.Completed() {
		return 100
	}
	index := e.Graph.prerequisites()
	var sum float64
	for _, t := range top {
		if slices.Contains(e.Done, t) {
			sum++
			continue
		}
		scope := append([]string{t}, closure(index, t)...)
		concluded := 0
		for _, id := range scope {
			if e.Nodes[id].Status != StatusUnchecked {
				concluded++
			}
		}
		share := float64(concluded) / float64(len(scope))
		sum += min(share, 0.99)
	}
	progress := int(sum / float64(len(top)) * 100)
	if e.MaxQuestions > 0 {
		// Asked already counts the question on screen; only answered ones
		// count, so a diagnostic starts at 0.
		answered := e.Asked
		if e.Current != nil && e.Current.Kind != KindClosing {
			answered--
		}
		progress = max(progress, answered*100/e.MaxQuestions)
	}
	return min(99, progress)
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

// WeakSpots are the topics passed on C: credited, but answered only in part,
// so the student sees what to strengthen besides the gaps.
func (e *Exam) WeakSpots() []string {
	out := []string{}
	for _, n := range e.Graph.Nodes {
		if g := e.Nodes[n.ID].Grade; g != nil && *g == GradeC {
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
