package application

import (
	"context"
)

type SearchFilter struct {
	OutcomeID, CompetencyID, ConstituentID        string
	TaxonomyCode, ALDLevelCode, TopicLevelCode    string
	Origin, SectionCode, CurriculumCompetencyCode string
	ImportanceMin, ImportanceMax                  int32
	Importance, Limit                             int32
	IncludeInTest                                 *bool
}

type TaskSummary struct {
	ID               string   `json:"id"`
	Question         string   `json:"question"`
	Origin           string   `json:"origin"`
	Options          []string `json:"options"`
	VoiceInstruction *string  `json:"voice_instruction" extensions:"x-nullable"`
	OutcomeID        string   `json:"outcome_id"`
	OutcomeName      string   `json:"outcome_name"`
	CompetencyName   string   `json:"competency_name"`
	ConstituentName  string   `json:"constituent_name"`
	TaxonomyCode     *string  `json:"taxonomy_code" extensions:"x-nullable"`
	ALDLevelCode     *string  `json:"ald_level_code" extensions:"x-nullable"`
	TopicLevelCode   *string  `json:"topic_level_code" extensions:"x-nullable"`
	Importance       *int16   `json:"importance" extensions:"x-nullable"`
	IncludeInTest    *bool    `json:"include_in_test" extensions:"x-nullable"`
}

type CurriculumSection struct {
	Code                   string   `json:"code"`
	Title                  string   `json:"title"`
	CurriculumCompetencies []string `json:"curriculum_competencies"`
}

type TaskProfile struct {
	ID                 string              `json:"id"`
	Question           string              `json:"question"`
	Origin             string              `json:"origin"`
	Options            []string            `json:"options"`
	VoiceInstruction   *string             `json:"voice_instruction" extensions:"x-nullable"`
	OutcomeID          string              `json:"outcome_id"`
	OutcomeName        string              `json:"outcome_name"`
	IncludeInTest      *bool               `json:"include_in_test" extensions:"x-nullable"`
	TaxonomyCode       *string             `json:"taxonomy_code" extensions:"x-nullable"`
	ALDLevelCode       *string             `json:"ald_level_code" extensions:"x-nullable"`
	Importance         *int16              `json:"importance" extensions:"x-nullable"`
	EducationalContent *string             `json:"educational_content" extensions:"x-nullable"`
	ConstituentID      string              `json:"constituent_id"`
	ConstituentName    string              `json:"constituent_name"`
	TopicLevelCode     *string             `json:"topic_level_code" extensions:"x-nullable"`
	CompetencyID       string              `json:"competency_id"`
	CompetencyName     string              `json:"competency_name"`
	CurriculumSections []CurriculumSection `json:"curriculum_sections"`
}

type Repository interface {
	Search(context.Context, SearchFilter) ([]TaskSummary, error)
	Profile(context.Context, string) (TaskProfile, error)
}

type Service struct{ repo Repository }

func New(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Search(ctx context.Context, filter SearchFilter) ([]TaskSummary, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.repo.Search(ctx, filter)
}

func (s *Service) Profile(ctx context.Context, taskID string) (TaskProfile, error) {
	return s.repo.Profile(ctx, taskID)
}
