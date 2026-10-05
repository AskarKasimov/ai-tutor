// Package competencymap describes an imported competency map.
package competencymap

type Competency struct{ Key, Name string }
type CurriculumSection struct {
	Code, Title     string
	CompetencyCodes []string
}
type Constituent struct {
	Key, CompetencyKey, Name string
	TopicLevelCode           string
	Sections                 []CurriculumSection
}
type Outcome struct {
	Key, ConstituentKey, Name  string
	IncludeInTest              *bool
	TaxonomyCode, ALDLevelCode string
	Importance                 *int
	EducationalContent         *string
	SourceRowIndexes           []int
}
type Task struct {
	OutcomeKey, Question, Criteria, ReferenceAnswer, Column, VoiceInstruction string
	Options                                                                   []string
	Row, SourceRowIndex, SourceColumnIndex                                    int
}
type SourceRow struct {
	Index, Line int
	Cells       []string
}
type ImportWarning struct {
	Row, ColumnIndex int
	Column, Code     string
}
type Map struct {
	Competencies      []Competency
	Constituents      []Constituent
	Outcomes          []Outcome
	Tasks             []Task
	UnparsedTaskCells int
	Warnings          []ImportWarning
	SourceFormat      string
	SourceHeaders     []string
	SourceRows        []SourceRow
}
type ImportResult struct {
	Revision          int64
	ImportedAt        int64
	CompetencyCount   int
	ConstituentCount  int
	OutcomeCount      int
	TaskCount         int
	UnparsedTaskCells int
	Warnings          []ImportWarning
}
