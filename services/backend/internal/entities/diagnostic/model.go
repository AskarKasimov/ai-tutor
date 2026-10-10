// Package diagnostic contains process-local diagnostic session state.
package diagnostic

type LearningState struct {
	SubjectID           string `json:"subject_id"`
	SubjectName         string `json:"subject_name"`
	DiagnosticSessionID string `json:"diagnostic_session_id,omitempty" binding:"optional"`
	ActiveSessionID     string `json:"active_session_id,omitempty" binding:"optional"`
	DiagnosticStatus    string `json:"diagnostic_status"`
	DiagnosticCompleted bool   `json:"diagnostic_completed"`
	TrainingAvailable   bool   `json:"training_available"`
}

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

func (s Session) Current() *TaskSnapshot {
	if s.Status != StatusActive || s.CurrentCompetency < 0 || s.CurrentCompetency >= len(s.Variant.Competencies) {
		return nil
	}
	competency := s.Variant.Competencies[s.CurrentCompetency]
	if s.CurrentTask < 0 || s.CurrentTask >= len(competency.Tasks) {
		return nil
	}
	task := competency.Tasks[s.CurrentTask]
	return &task
}

func (s Session) Progress() Progress {
	return Progress{
		SessionID: s.ID, Status: s.Status, Current: s.Current(),
		Completed: len(s.Answers), Skipped: len(s.SkippedBasics), Total: s.Variant.TaskCount(),
	}
}

type VariantSnapshot struct {
	ID                      string
	SubjectID               string
	SubjectNameSnapshot     string
	MapRevision             int64
	AlgorithmVersion        string
	IncludedCompetencyCount int
	SkippedCompetencies     []SkippedCompetency
	Competencies            []Competency
}

// TaskCount includes only positions present in the saved variant.
func (v VariantSnapshot) TaskCount() int {
	total := 0
	for _, competency := range v.Competencies {
		total += len(competency.Tasks)
	}
	return total
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
	AudioAssetID       *string
	ReferenceAnswer    string
	Criteria           string
	TaxonomyCode       string
	ALDLevelCode       string
	Importance         int16
	IncludeInTest      bool
	EducationalContent string
}

type Answer struct {
	Skipped          bool
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
	AnswerSkipped    bool
	Text             string
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
