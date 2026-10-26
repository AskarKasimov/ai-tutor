package competencymap

// Snapshot is the current map's public hierarchy, without grading secrets.
type Snapshot struct {
	Revision     int64
	ImportedAt   *int64
	Competencies []CompetencyNode
}

type CompetencyNode struct {
	ID, Name     string
	Constituents []ConstituentNode
}

type ConstituentNode struct {
	ID, Name       string
	TopicLevelCode *string
	Sections       []CurriculumSection
	Outcomes       []OutcomeNode
}

type OutcomeNode struct {
	ID, Name                   string
	IncludeInTest              *bool
	TaxonomyCode, ALDLevelCode *string
	Importance                 *int16
	EducationalContent         *string
	Tasks                      []TaskNode
}

type TaskNode struct {
	ID, Question, Origin string
	Options              []string
	VoiceInstruction     *string
}
