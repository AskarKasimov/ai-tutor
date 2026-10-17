package exam

import (
	"slices"
	"strings"
)

// Session is one diagnostic of a student on a course. Version guards
// concurrent updates.
type Session struct {
	ID          string        `json:"id"`
	OwnerID     string        `json:"owner_id"`
	SubjectID   string        `json:"subject_id"`
	Status      SessionStatus `json:"status"`
	StartedAt   int64         `json:"started_at"`
	CompletedAt *int64        `json:"completed_at"`
	Version     int           `json:"version"`
	Exam        *Exam         `json:"exam"`
	Turns       []Turn        `json:"turns"`
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

// Result is the student's outcome of a completed diagnostic.
type Result struct {
	SessionID    string
	Grade        *Grade
	ScorePercent *int
	Nodes        []NodeResult
	Gaps         []Gap
}

func (s Session) Result() Result {
	e := s.Exam
	top := e.Graph.TopLevel()
	r := Result{SessionID: s.ID, Nodes: []NodeResult{}, Gaps: []Gap{}}
	r.Grade, r.ScorePercent = e.CourseGrade()
	for _, n := range e.Graph.Nodes {
		state := e.Nodes[n.ID]
		r.Nodes = append(r.Nodes, NodeResult{NodeID: n.ID, Title: n.Title, Status: state.Status, Grade: state.Grade, TopLevel: slices.Contains(top, n.ID)})
	}
	for _, id := range e.Gaps() {
		n, _ := e.Graph.Node(id)
		r.Gaps = append(r.Gaps, NewGap(id, n.Title, n.Sources))
	}
	return r
}
