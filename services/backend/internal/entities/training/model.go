package training

import "time"

type Task struct {
	SourceTaskID       string
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
	EducationalContent string
	Importance         int16
	AudioAssetID       *string
}
type Target struct {
	Kind             string
	Source           Task
	Sources          []Task
	OriginalScore    int
	OriginalMaxScore int
	LastScore        *int
	OriginalFeedback []string
	OriginalCriteria []CriterionResult
}
type Exercise struct {
	ID              string
	TargetIndex     int
	Round           int64
	ProviderVersion string
	Task            Task
}
type Session struct {
	ID              string
	OwnerID         string
	DiagnosticID    string
	SubjectID       string
	SubjectName     string
	Mode            string
	PlanRevision    int64
	DiagnosticScore int
	MaximumScore    int
	Round           int64
	AnswerCount     int64
	Targets         []Target
	Current         Exercise
}
type Reservation struct {
	Key             string
	Fingerprint     string
	Token           string
	ExerciseID      string
	LeaseUntil      time.Time
	TranscriptionID string
	Text            string
}
type CriterionResult struct {
	Key         string `json:"key"`
	Satisfied   bool   `json:"satisfied"`
	Explanation string `json:"explanation"`
}
type Attempt struct {
	Sequence         int64             `json:"sequence"`
	ExerciseID       string            `json:"exercise_id"`
	Round            int64             `json:"round"`
	TargetIndex      int               `json:"target_index"`
	TranscriptionID  string            `json:"transcription_id"`
	Text             string            `json:"text"`
	Score            int               `json:"score"`
	MaxScore         int               `json:"max_score"`
	Verdict          string            `json:"verdict"`
	CriterionResults []CriterionResult `json:"criterion_results"`
	Feedback         []string          `json:"feedback"`
	CreatedAt        int64             `json:"created_at"`
}
type TargetView struct {
	Kind             string `json:"kind"`
	Label            string `json:"label"`
	CompetencyID     string `json:"competency_id"`
	CompetencyName   string `json:"competency_name"`
	OutcomeID        string `json:"outcome_id"`
	OutcomeName      string `json:"outcome_name"`
	OriginalScore    *int   `json:"original_score" extensions:"x-nullable"`
	OriginalMaxScore *int   `json:"original_max_score" extensions:"x-nullable"`
	LastScore        *int   `json:"last_score" extensions:"x-nullable"`
}
type ExerciseView struct {
	ID               string   `json:"exercise_id"`
	OutcomeID        string   `json:"outcome_id"`
	OutcomeName      string   `json:"outcome_name"`
	Question         string   `json:"question"`
	Options          []string `json:"options"`
	VoiceInstruction string   `json:"voice_instruction"`
}
type Progress struct {
	ID           string       `json:"session_id"`
	DiagnosticID string       `json:"diagnostic_session_id"`
	SubjectID    string       `json:"subject_id"`
	SubjectName  string       `json:"subject_name"`
	Mode         string       `json:"mode"`
	Status       string       `json:"status"`
	Round        int64        `json:"round"`
	AnswerCount  int64        `json:"answer_count"`
	Targets      []TargetView `json:"targets"`
	Current      ExerciseView `json:"current"`
	Answer       *Attempt     `json:"answer,omitempty" binding:"optional"`
}
type Preview struct {
	DiagnosticID        string       `json:"diagnostic_session_id"`
	SubjectID           string       `json:"subject_id"`
	SubjectName         string       `json:"subject_name"`
	Mode                string       `json:"mode"`
	PlanRevision        int64        `json:"plan_revision"`
	DiagnosticScore     int          `json:"diagnostic_score"`
	MaximumScore        int          `json:"maximum_score"`
	Status              string       `json:"status"`
	ConfirmedGaps       []TargetView `json:"confirmed_gaps"`
	PartialCompetencies []TargetView `json:"partial_competencies"`
	Topics              []TargetView `json:"topics"`
}

func View(t Target) TargetView {
	label := "Свободная тренировка"
	if t.Kind == "confirmed_gap" {
		label = "Неосвоенные темы"
	}
	if t.Kind == "partial_competency" {
		label = "Частичные пробелы в знаниях"
	}
	var score, maxScore *int
	if t.OriginalMaxScore > 0 {
		score = &t.OriginalScore
		maxScore = &t.OriginalMaxScore
	}
	return TargetView{Kind: t.Kind, Label: label, CompetencyID: t.Source.CompetencyID, CompetencyName: t.Source.CompetencyName, OutcomeID: t.Source.OutcomeID, OutcomeName: t.Source.OutcomeName, OriginalScore: score, OriginalMaxScore: maxScore, LastScore: t.LastScore}
}
func (s Session) Progress() Progress {
	targets := make([]TargetView, 0, len(s.Targets))
	for _, t := range s.Targets {
		targets = append(targets, View(t))
	}
	t := s.Current.Task
	return Progress{ID: s.ID, DiagnosticID: s.DiagnosticID, SubjectID: s.SubjectID, SubjectName: s.SubjectName, Mode: s.Mode, Status: "active", Round: s.Round, AnswerCount: s.AnswerCount, Targets: targets, Current: ExerciseView{ID: s.Current.ID, OutcomeID: t.OutcomeID, OutcomeName: t.OutcomeName, Question: t.Question, Options: append([]string{}, t.Options...), VoiceInstruction: t.VoiceInstruction}}
}
