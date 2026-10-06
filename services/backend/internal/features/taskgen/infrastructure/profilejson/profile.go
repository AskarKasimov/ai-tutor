// Package profilejson maps generation profiles to the JSON representation used
// by model requests and stored generation snapshots.
package profilejson

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"

type Outcome struct {
	ID, Name, CompetencyName, ConstituentName  string
	IncludeInTest                              *bool
	TaxonomyCode, ALDLevelCode, TopicLevelCode *string
	Importance                                 *int16
	EducationalContent                         *string
	CurriculumSections                         []CurriculumSection
}

type CurriculumSection struct {
	Code                   string   `json:"code"`
	Title                  string   `json:"title"`
	CurriculumCompetencies []string `json:"curriculum_competencies"`
}

func FromOutcome(outcome application.Outcome) Outcome {
	sections := make([]CurriculumSection, len(outcome.CurriculumSections))
	for i, section := range outcome.CurriculumSections {
		sections[i] = CurriculumSection{Code: section.Code, Title: section.Title, CurriculumCompetencies: section.CurriculumCompetencies}
	}
	return Outcome{
		ID: outcome.ID, Name: outcome.Name, CompetencyName: outcome.CompetencyName, ConstituentName: outcome.ConstituentName,
		IncludeInTest: outcome.IncludeInTest, TaxonomyCode: outcome.TaxonomyCode, ALDLevelCode: outcome.ALDLevelCode,
		TopicLevelCode: outcome.TopicLevelCode, Importance: outcome.Importance, EducationalContent: outcome.EducationalContent,
		CurriculumSections: sections,
	}
}

func ToSections(sections []CurriculumSection) []application.CurriculumSection {
	result := make([]application.CurriculumSection, len(sections))
	for i, section := range sections {
		result[i] = application.CurriculumSection{Code: section.Code, Title: section.Title, CurriculumCompetencies: section.CurriculumCompetencies}
	}
	return result
}
