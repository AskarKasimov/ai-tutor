// Package competencymap describes an imported competency map.
package competencymap

type Competency struct{ Key, Name string }
type Constituent struct{ Key, CompetencyKey, Name string }
type Outcome struct {
	Key, ConstituentKey, Name string
	Attributes                []map[string]string
}
type Task struct {
	OutcomeKey, Question, Criteria, Column string
	Row                                    int
}
type Map struct {
	Competencies []Competency
	Constituents []Constituent
	Outcomes     []Outcome
	Tasks        []Task
}
type ImportResult struct {
	Revision         int64
	ImportedAt       int64
	CompetencyCount  int
	ConstituentCount int
	OutcomeCount     int
	TaskCount        int
}
