package exam

import "slices"

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
	NodeID string
	Title  string
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
		r.Gaps = append(r.Gaps, Gap{NodeID: id, Title: n.Title})
	}
	return r
}
