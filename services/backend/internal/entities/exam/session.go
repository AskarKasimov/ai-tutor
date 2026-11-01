package exam

import (
	"slices"
	"strings"
)

// Session is one diagnostic of a student on a course. Version guards
// concurrent updates.
type Session struct {
	AddressPreference AddressPreference `json:"address_preference,omitempty"`
	ID                string            `json:"id"`
	OwnerID           string            `json:"owner_id"`
	SubjectID         string            `json:"subject_id"`
	Status            SessionStatus     `json:"status"`
	StartedAt         int64             `json:"started_at"`
	CompletedAt       *int64            `json:"completed_at"`
	Version           int               `json:"version"`
	Exam              *Exam             `json:"exam"`
	Turns             []Turn            `json:"turns"`
}

// AddressPreference records only an explicitly requested grammatical form.
type AddressPreference string

const (
	AddressFeminine  AddressPreference = "feminine"
	AddressMasculine AddressPreference = "masculine"
	AddressNeutral   AddressPreference = "neutral"
)

func (p AddressPreference) Valid() bool {
	return p == "" || p == AddressFeminine || p == AddressMasculine || p == AddressNeutral
}

// CurrentTurn is the Geek turn awaiting an answer; empty unless active.
func (s Session) CurrentTurn() (Turn, bool) {
	if s.Status != SessionActive || len(s.Turns) == 0 {
		return Turn{}, false
	}
	last := s.Turns[len(s.Turns)-1]
	return last, last.Speaker == SpeakerGeek && last.Kind != KindClosing
}

// Turn returns a turn by ID.
func (s Session) Turn(id string) (Turn, bool) {
	i := slices.IndexFunc(s.Turns, func(t Turn) bool { return t.ID == id })
	if i < 0 {
		return Turn{}, false
	}
	return s.Turns[i], true
}

// Parts splits a Geek turn into the reply to the previous answer and the
// question or closing after it. Turns saved before Reply existed have none.
func (t Turn) Parts() (reply, question string) {
	if t.Reply == "" || !strings.HasPrefix(t.Text, t.Reply) {
		return "", strings.TrimSpace(t.Text)
	}
	return strings.TrimSpace(t.Reply), strings.TrimSpace(strings.TrimPrefix(t.Text, t.Reply))
}

// Dialogue returns the turns about one topic, in order.
func (s Session) Dialogue(nodeID string) []Turn {
	out := []Turn{}
	for _, t := range s.Turns {
		if t.NodeID != nil && *t.NodeID == nodeID {
			out = append(out, t)
		}
	}
	return out
}

type NodeResult struct {
	NodeID   string
	Title    string
	Status   Status
	Grade    *Grade
	TopLevel bool
	// Requires lists the node's direct prerequisites in this graph snapshot.
	Requires []string
	// Assisted tells that the student answered after a rephrase or a hint.
	Assisted bool
	// Score and Maximum are the topic's points; nil when it has none.
	Score   *int
	Maximum *int
	Marks   []TopicMark
	// Reply is what Geek said when the topic closed; empty when unknown.
	Reply string
}

type Gap struct {
	NodeID            string
	Title             string
	Weeks             []int32
	Sources           []SourceCitation
	LearningMaterials LearningMaterials
}

type MaterialReference struct {
	MaterialID string  `json:"material_id"`
	Filename   *string `json:"filename"`
	Kind       *string `json:"kind"`
	KindLabel  *string `json:"kind_label"`
	Week       *int32  `json:"week"`
	Revision   *string `json:"material_revision"`
}

type LearningMaterials struct {
	Theory     []MaterialReference `json:"theory"`
	Practice   []MaterialReference `json:"practice"`
	Additional []MaterialReference `json:"additional"`
}

// NewGap derives the stable source list and study plan from this graph snapshot.
func NewGap(id, title string, sources []SourceCitation) Gap {
	weeks := []int32{}
	seenWeeks := map[int32]bool{}
	for _, source := range sources {
		if source.Week != nil && !seenWeeks[*source.Week] {
			weeks = append(weeks, *source.Week)
			seenWeeks[*source.Week] = true
		}
	}
	slices.Sort(weeks)
	materials := SelectLearningMaterials(sources)
	return Gap{NodeID: id, Title: title, Weeks: weeks, Sources: append([]SourceCitation{}, sources...), LearningMaterials: materials}
}

// closingReplies are Geek's replies that closed a topic. Such a reply opens the
// turn after the topic, so it belongs to the topic of the last student answer.
func (s Session) closingReplies() map[string]string {
	replies := map[string]string{}
	last := ""
	for _, t := range s.Turns {
		switch {
		case t.Speaker == SpeakerStudent && t.NodeID != nil:
			last = *t.NodeID
		case t.Speaker == SpeakerGeek && t.Recap != "" && last != "":
			replies[last] = strings.TrimSpace(t.Recap)
		}
	}
	return replies
}

// restoreDontKnow marks the topics of a session saved before NodeState.DontKnow:
// a topic that closed with zeros right after the "don't know" button had no
// answer to score, whatever an earlier attempt left in its state.
func (s Session) restoreDontKnow() {
	last := map[string]*Source{}
	for _, t := range s.Turns {
		if t.Speaker == SpeakerStudent && t.NodeID != nil {
			last[*t.NodeID] = t.Source
		}
	}
	for id, source := range last {
		state, ok := s.Exam.Nodes[id]
		if ok && source != nil && *source == SourceDontKnow && state.Status == StatusGap && state.Criteria != nil && *state.Criteria == (Criteria{}) {
			state.DontKnow = true
		}
	}
}

// Result is the student's outcome of a completed diagnostic.
type Result struct {
	SessionID    string
	Grade        *Grade
	ScorePercent *int
	// Score and Maximum are the points of all asked topics.
	Score   int
	Maximum int
	Nodes   []NodeResult
	Gaps    []Gap
	// WeakSpots are the topics on C, with the same sources as a gap.
	WeakSpots []Gap
}

func (s Session) Result() Result {
	s.restoreDontKnow()
	e := s.Exam
	top := e.Graph.TopLevel()
	r := Result{SessionID: s.ID, Nodes: []NodeResult{}, Gaps: []Gap{}, WeakSpots: []Gap{}}
	replies := s.closingReplies()
	for _, n := range e.Graph.Nodes {
		state := e.Nodes[n.ID]
		marks := e.TopicMarks(n.ID)
		node := NodeResult{NodeID: n.ID, Title: n.Title, Status: state.Status, Grade: state.Grade, TopLevel: slices.Contains(top, n.ID),
			Requires: append([]string{}, n.Prerequisites...), Assisted: state.Assisted, Marks: marks, Reply: replies[n.ID]}
		if len(marks) > 0 {
			score, maximum := 0, 0
			for _, m := range marks {
				score += m.Score
				maximum += m.Maximum
			}
			node.Score, node.Maximum = &score, &maximum
			r.Score += score
			r.Maximum += maximum
		}
		r.Nodes = append(r.Nodes, node)
	}
	r.Grade, r.ScorePercent = CourseGrade(r.Score, r.Maximum)
	for _, id := range e.Gaps() {
		n, _ := e.Graph.Node(id)
		r.Gaps = append(r.Gaps, NewGap(id, n.Title, n.Sources))
	}
	for _, id := range e.WeakSpots() {
		n, _ := e.Graph.Node(id)
		r.WeakSpots = append(r.WeakSpots, NewGap(id, n.Title, n.Sources))
	}
	return r
}
