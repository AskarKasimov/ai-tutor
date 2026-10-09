// Package variant contains immutable variant and task snapshots.
package variant

import "context"

// TaskReader exposes private task data to server-side grading after checking ownership.
type TaskReader interface {
	TaskForGrading(ctx context.Context, ownerID, variantID, variantTaskID string) (VariantTask, error)
}

type Variant struct {
	ID                      string
	OwnerID                 string
	SubjectID               string
	SubjectNameSnapshot     string
	MapRevision             int64
	AlgorithmVersion        string
	IncludedCompetencyCount int
	TaskCount               int
	SkippedCompetencies     []SkippedCompetency
	CreatedAt               int64
	Competencies            []CompetencySelection
}

type CompetencySelection struct {
	Position   int
	Competency CompetencyProfile
	Tasks      []VariantTask
}

type SkippedCompetency struct {
	CompetencyID       string `json:"competency_id"`
	CompetencyName     string `json:"competency_name"`
	Code               string `json:"code"`
	EligibleOutcomes   int    `json:"eligible_outcome_count"`
	LowerBloomOutcomes int    `json:"lower_bloom_outcome_count,omitempty"`
	MainBloomRank      int    `json:"main_bloom_rank,omitempty"`
}

type CompetencyProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ConstituentProfile struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	TopicLevelCode *string             `json:"topic_level_code"`
	Sections       []CurriculumSection `json:"sections"`
}

type CurriculumSection struct {
	Code                   string   `json:"code"`
	Title                  string   `json:"title"`
	CurriculumCompetencies []string `json:"curriculum_competencies"`
}

type OutcomeProfile struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	IncludeInTest      *bool   `json:"include_in_test"`
	TaxonomyCode       *string `json:"taxonomy_code"`
	ALDLevelCode       *string `json:"ald_level_code"`
	Importance         *int16  `json:"importance"`
	EducationalContent *string `json:"educational_content"`
}

type TaskProfile struct {
	ID                string             `json:"id"`
	Question          string             `json:"question"`
	Options           []string           `json:"options"`
	VoiceInstruction  *string            `json:"voice_instruction"`
	AudioAssetID      *string            `json:"audio_asset_id"`
	ReferenceAnswer   *string            `json:"reference_answer"`
	Criteria          *string            `json:"criteria"`
	Origin            string             `json:"origin"`
	CreatedAt         int64              `json:"created_at"`
	SourceRowIndex    *int32             `json:"source_row_index"`
	SourceColumnIndex *int32             `json:"source_column_index"`
	Competency        CompetencyProfile  `json:"competency"`
	Constituent       ConstituentProfile `json:"constituent"`
	Outcome           OutcomeProfile     `json:"outcome"`
}

type VariantTask struct {
	ID   string      `json:"id"`
	Role string      `json:"role"`
	Task TaskProfile `json:"task"`
}

type CandidateOutcome struct {
	Profile               TaskProfile
	CompetencySourceOrder int64
	SourceOrder           int64
	HasTask               bool
}
