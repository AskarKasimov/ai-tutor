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
	ID               string
	Question         string
	Origin           string
	Options          []string
	VoiceInstruction *string
	OutcomeID        string
	OutcomeName      string
	CompetencyName   string
	ConstituentName  string
	TaxonomyCode     *string
	ALDLevelCode     *string
	TopicLevelCode   *string
	Importance       *int16
	IncludeInTest    *bool
}

type CurriculumSection struct {
	Code                   string
	Title                  string
	CurriculumCompetencies []string
}

type TaskProfile struct {
	ID                 string
	Question           string
	Origin             string
	Options            []string
	VoiceInstruction   *string
	OutcomeID          string
	OutcomeName        string
	IncludeInTest      *bool
	TaxonomyCode       *string
	ALDLevelCode       *string
	Importance         *int16
	EducationalContent *string
	ConstituentID      string
	ConstituentName    string
	TopicLevelCode     *string
	CompetencyID       string
	CompetencyName     string
	CurriculumSections []CurriculumSection
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
