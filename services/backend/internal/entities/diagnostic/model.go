// Package diagnostic contains process-local diagnostic session state.
package diagnostic

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

type Session struct {
	ID                 string
	OwnerID            string
	VariantID          string
	Status             string
	CurrentCompetency  int
	CurrentTask        int
	Variant            VariantSnapshot
	Answers            []Answer
	SkippedBasics      []TaskSnapshot
	AcceptedRequests   map[string]AcceptedRequest
	StartRequestDigest string
	InFlight           *Reservation
}

type VariantSnapshot struct {
	ID                      string
	MapRevision             int64
	AlgorithmVersion        string
	IncludedCompetencyCount int
	SkippedCompetencies     []SkippedCompetency
	Competencies            []Competency
}

type Competency struct {
	ID       string
	Name     string
	Position int
	Tasks    []TaskSnapshot
}

type TaskSnapshot struct {
	ID                 string
	SourceTaskID       string
	Role               string
	CompetencyID       string
	CompetencyName     string
	ConstituentID      string
	ConstituentName    string
	OutcomeID          string
	OutcomeName        string
	Question           string
	Options            []string
	VoiceInstruction   string
	ReferenceAnswer    string
	Criteria           string
	TaxonomyCode       string
	ALDLevelCode       string
	Importance         int16
	IncludeInTest      bool
	EducationalContent string
}

type Answer struct {
	VariantTaskID    string
	SourceTaskID     string
	CompetencyID     string
	OutcomeID        string
	Role             string
	Task             TaskSnapshot
	TranscriptionID  string
	Text             string
	GraderScore      int
	GraderMaxScore   int
	Score            int
	Verdict          string
	CriterionResults []CriterionResult
	Feedback         []string
	CreatedAt        int64
}

type CriterionResult struct {
	Key         string
	Satisfied   bool
	Explanation string
}

type SkippedCompetency struct {
	ID   string
	Name string
	Code string
}

type AcceptedRequest struct {
	Fingerprint string
	Answer      Answer
	Response    Progress
}

type Transition struct {
	CurrentCompetency int
	CurrentTask       int
	Status            string
	SkippedBasics     []TaskSnapshot
}

type Reservation struct {
	Key               string
	Fingerprint       string
	VariantTaskID     string
	Token             string
	TranscriptionID   string
	TranscriptionText string
}

type Progress struct {
	SessionID        string
	Status           string
	Completed        int
	Skipped          int
	Total            int
	Current          *TaskSnapshot
	Score            *int
	GraderScore      *int
	GraderMaxScore   *int
	Verdict          string
	CriterionResults []CriterionResult
	Feedback         []string
}

type Result struct {
	SessionID               string
	Status                  string
	VariantID               string
	MapRevision             int64
	IncludedCompetencyCount int
	SkippedCompetencies     []SkippedCompetency
	CompletedTasks          int
	TotalTasks              int
	DiagnosticScore         int
	MaximumScore            int
	Answers                 []Answer
	UntestedBasics          []TaskSnapshot
}
