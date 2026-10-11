package training

import "strings"

// Stages of a competency block. A block starts with its main ("bright") task.
// A failed main task opens a theory summary built from every ОС of the
// competency, then two basic ("dim") tasks of the same competency follow.
// Failed basics repeat theory and basics; solved basics return to the main
// task; a solved main task moves the student to the next block.
const (
	StageMain   = "main"
	StageTheory = "theory"
	StageBasic  = "basic"
	// StagePractice marks exercises of sessions created before blocks existed.
	StagePractice = "practice"
)

// TheoryNote is the immutable ОС text of one outcome. It includes outcomes
// that are not marked TRUE for the test.
type TheoryNote struct {
	OutcomeID     string
	OutcomeName   string
	IncludeInTest bool
	Content       string
}

// Block is the frozen material and progress of one competency.
type Block struct {
	Main   []Task
	Basics [][]Task
	Notes  []TheoryNote
	// Theory is the generated summary cached after its first successful generation.
	Theory      string
	Mastered    bool
	Cycles      int
	MainShown   int
	BasicShown  []int
	BasicScores []int
}

// Material is the active subject bank read once when a session is planned.
type Material struct {
	Revision int64
	// Tasks are ready tasks of every outcome in source order.
	Tasks []Task
	// Notes are ОС texts by competency ID in source order, including FALSE outcomes.
	Notes map[string][]TheoryNote
}

// TheoryRequest is the trusted input of a theory summary.
type TheoryRequest struct {
	SubjectName    string
	CompetencyName string
	MainOutcome    string
	Notes          []TheoryNote
}

// Solved reports full credit for a training answer graded on the 0/1/2 scale.
func Solved(score int) bool { return score == 2 }

// Step is the next position inside the session.
type Step struct {
	TargetIndex int
	Stage       string
	BasicIndex  int
}

// Entry is the first position of a target: the main task of a block.
func (s Session) Entry(index int) Step {
	if s.Targets[index].Block == nil {
		return Step{TargetIndex: index}
	}
	return Step{TargetIndex: index, Stage: StageMain}
}

// Advance records the result of the current exercise in the block state and
// returns the next position. The score is ignored for theory. Sessions without
// blocks keep the original cycle over targets.
func (s *Session) Advance(score int) Step {
	current := s.Current
	nextBlock := func() Step {
		next := current.TargetIndex + 1
		if next >= len(s.Targets) {
			next = 0
			s.Round++
		}
		return s.Entry(next)
	}
	block := s.Targets[current.TargetIndex].Block
	if block == nil {
		return nextBlock()
	}
	theory := func() Step {
		block.Cycles++
		block.BasicScores = nil
		return Step{TargetIndex: current.TargetIndex, Stage: StageTheory}
	}
	switch current.Stage {
	case StageTheory:
		block.BasicScores = nil
		if len(block.Basics) == 0 {
			return Step{TargetIndex: current.TargetIndex, Stage: StageMain}
		}
		return Step{TargetIndex: current.TargetIndex, Stage: StageBasic}
	case StageBasic:
		block.BasicScores = append(block.BasicScores, score)
		if current.BasicIndex+1 < len(block.Basics) {
			return Step{TargetIndex: current.TargetIndex, Stage: StageBasic, BasicIndex: current.BasicIndex + 1}
		}
		for _, value := range block.BasicScores {
			if !Solved(value) {
				return theory()
			}
		}
		block.BasicScores = nil
		return Step{TargetIndex: current.TargetIndex, Stage: StageMain}
	default:
		if Solved(score) {
			block.Mastered = true
			return nextBlock()
		}
		return theory()
	}
}

// FallbackTheory joins the source ОС texts when the summary model is unavailable.
func FallbackTheory(notes []TheoryNote) string {
	parts := make([]string, 0, len(notes))
	for _, note := range notes {
		content := strings.TrimSpace(note.Content)
		if content == "" {
			continue
		}
		parts = append(parts, note.OutcomeName+"\n"+content)
	}
	if len(parts) == 0 {
		return "Для этой темы в карте пока нет учебного содержания. Задайте репетитору вопросы по теме, затем переходите к закрепляющим задачам."
	}
	return strings.Join(parts, "\n\n")
}

// Pick rotates a task pool by the number of times it was already shown.
func Pick(pool []Task, shown int) (Task, bool) {
	if len(pool) == 0 {
		return Task{}, false
	}
	return pool[shown%len(pool)], true
}
