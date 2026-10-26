package variantgenhttp

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
)

type CreateRequest struct {
	SubjectID string `json:"subject_id" minLength:"1" maxLength:"128" binding:"required"`
}

type SkippedCompetency struct {
	CompetencyID           string `json:"competency_id" minLength:"1" maxLength:"128" binding:"required"`
	CompetencyName         string `json:"competency_name"`
	EligibleOutcomeCount   int    `json:"eligible_outcome_count" minimum:"0" maximum:"101"`
	Code                   string `json:"code" enums:"NO_READY_OUTCOMES,INSUFFICIENT_DISTINCT_OUTCOMES,INSUFFICIENT_LOWER_BLOOM_OUTCOMES"`
	Message                string `json:"message"`
	LowerBloomOutcomeCount int    `json:"lower_bloom_outcome_count,omitempty" minimum:"0" maximum:"101" binding:"optional"`
	MainBloomRank          int    `json:"main_bloom_rank,omitempty" minimum:"1" maximum:"4" binding:"optional"`
}
type VariantTask struct {
	ID               string   `json:"id" minLength:"1" maxLength:"128"`
	SourceTaskID     string   `json:"source_task_id" minLength:"1" maxLength:"128"`
	CompetencyID     string   `json:"competency_id" minLength:"1" maxLength:"128"`
	CompetencyName   string   `json:"competency_name"`
	ConstituentID    string   `json:"constituent_id" minLength:"1" maxLength:"128"`
	ConstituentName  string   `json:"constituent_name"`
	OutcomeID        string   `json:"outcome_id" minLength:"1" maxLength:"128"`
	OutcomeName      string   `json:"outcome_name"`
	Role             string   `json:"role" enums:"main,basic"`
	TaxonomyCode     string   `json:"taxonomy_code" enums:"knowledge,understanding,application,analysis"`
	Importance       int16    `json:"importance" minimum:"1" maximum:"5"`
	IncludeInTest    bool     `json:"include_in_test" enums:"true"`
	Question         string   `json:"question" minLength:"1"`
	Options          []string `json:"options"`
	VoiceInstruction string   `json:"voice_instruction" minLength:"1"`
	Origin           string   `json:"origin" enums:"authored,ai_generated"`
}
type CompetencyBlock struct {
	CompetencyID   string        `json:"competency_id" minLength:"1" maxLength:"128"`
	CompetencyName string        `json:"competency_name"`
	Position       int           `json:"position" minimum:"1"`
	Main           VariantTask   `json:"main"`
	Basic          []VariantTask `json:"basic" minItems:"0" maxItems:"2"`
}
type Variant struct {
	ID                      string              `json:"id" minLength:"1" maxLength:"128"`
	SubjectID               string              `json:"subject_id" minLength:"1" maxLength:"128"`
	SubjectNameSnapshot     string              `json:"subject_name_snapshot" minLength:"1"`
	MapRevision             int64               `json:"map_revision" minimum:"1"`
	AlgorithmVersion        string              `json:"algorithm_version" minLength:"1" maxLength:"128"`
	IncludedCompetencyCount int                 `json:"included_competency_count" minimum:"1"`
	SkippedCompetencyCount  int                 `json:"skipped_competency_count" minimum:"0"`
	TaskCount               int                 `json:"task_count" minimum:"1"`
	CreatedAt               int64               `json:"created_at" minimum:"0"`
	Competencies            []CompetencyBlock   `json:"competencies" minItems:"1"`
	SkippedCompetencies     []SkippedCompetency `json:"skipped_competencies"`
}
type VariantSummary struct {
	ID                      string `json:"id" minLength:"1" maxLength:"128"`
	SubjectID               string `json:"subject_id" minLength:"1" maxLength:"128"`
	SubjectNameSnapshot     string `json:"subject_name_snapshot" minLength:"1"`
	MapRevision             int64  `json:"map_revision" minimum:"1"`
	AlgorithmVersion        string `json:"algorithm_version" minLength:"1" maxLength:"128"`
	IncludedCompetencyCount int    `json:"included_competency_count" minimum:"1"`
	SkippedCompetencyCount  int    `json:"skipped_competency_count" minimum:"0"`
	TaskCount               int    `json:"task_count" minimum:"1"`
	CreatedAt               int64  `json:"created_at" minimum:"0"`
}
type VariantList struct {
	Items      []VariantSummary `json:"items"`
	NextCursor *string          `json:"next_cursor" extensions:"x-nullable"`
}

func taskDTO(task variant.VariantTask) VariantTask {
	return VariantTask{ID: task.ID, SourceTaskID: task.Task.ID, CompetencyID: task.Task.Competency.ID,
		CompetencyName: task.Task.Competency.Name, ConstituentID: task.Task.Constituent.ID,
		ConstituentName: task.Task.Constituent.Name, OutcomeID: task.Task.Outcome.ID,
		OutcomeName: task.Task.Outcome.Name, Role: task.Role, TaxonomyCode: value(task.Task.Outcome.TaxonomyCode),
		Importance: valueInt16(task.Task.Outcome.Importance), IncludeInTest: valueBool(task.Task.Outcome.IncludeInTest),
		Question: task.Task.Question, Options: nonNil(task.Task.Options), VoiceInstruction: value(task.Task.VoiceInstruction), Origin: task.Task.Origin}
}

func variantDTO(value variant.Variant) Variant {
	blocks := make([]CompetencyBlock, 0, len(value.Competencies))
	for _, selection := range value.Competencies {
		if len(selection.Tasks) < 1 || len(selection.Tasks) > 3 {
			continue
		}
		block := CompetencyBlock{CompetencyID: selection.Competency.ID, CompetencyName: selection.Competency.Name,
			Position: selection.Position, Main: taskDTO(selection.Tasks[0]), Basic: make([]VariantTask, 0, len(selection.Tasks)-1)}
		for _, basic := range selection.Tasks[1:] {
			block.Basic = append(block.Basic, taskDTO(basic))
		}
		blocks = append(blocks, block)
	}
	skipped := make([]SkippedCompetency, 0, len(value.SkippedCompetencies))
	for _, item := range value.SkippedCompetencies {
		skipped = append(skipped, SkippedCompetency{CompetencyID: item.CompetencyID, CompetencyName: item.CompetencyName,
			EligibleOutcomeCount: item.EligibleOutcomes, Code: item.Code, Message: skipMessage(item),
			LowerBloomOutcomeCount: item.LowerBloomOutcomes, MainBloomRank: item.MainBloomRank})
	}
	return Variant{ID: value.ID, SubjectID: value.SubjectID, SubjectNameSnapshot: value.SubjectNameSnapshot, MapRevision: value.MapRevision, AlgorithmVersion: value.AlgorithmVersion,
		IncludedCompetencyCount: value.IncludedCompetencyCount, SkippedCompetencyCount: len(skipped),
		TaskCount: value.TaskCount, CreatedAt: value.CreatedAt, Competencies: blocks, SkippedCompetencies: skipped}
}

func summaryDTO(value variant.Variant) VariantSummary {
	return VariantSummary{ID: value.ID, SubjectID: value.SubjectID, SubjectNameSnapshot: value.SubjectNameSnapshot, MapRevision: value.MapRevision, AlgorithmVersion: value.AlgorithmVersion,
		IncludedCompetencyCount: value.IncludedCompetencyCount, SkippedCompetencyCount: len(value.SkippedCompetencies),
		TaskCount: value.TaskCount, CreatedAt: value.CreatedAt}
}

func listDTO(items []variant.Variant, cursor string) VariantList {
	result := VariantList{Items: make([]VariantSummary, 0, len(items))}
	for _, item := range items {
		result.Items = append(result.Items, summaryDTO(item))
	}
	if cursor != "" {
		result.NextCursor = &cursor
	}
	return result
}

func skipMessage(item variant.SkippedCompetency) string {
	if item.Code == "NO_READY_OUTCOMES" {
		return "Нет готовых TRUE-ОР с допустимыми таксономией и важностью."
	}
	if item.Code == "INSUFFICIENT_LOWER_BLOOM_OUTCOMES" {
		return "Недостаточно разных готовых TRUE-ОР с рангом Блума ниже основного."
	}
	return "Меньше трёх разных готовых TRUE-ОР."
}
func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func valueInt16(p *int16) int16 {
	if p == nil {
		return 0
	}
	return *p
}
func valueBool(p *bool) bool { return p != nil && *p }
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
