package taskbankhttp

import "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/application"

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

func taskSummaryDTO(task application.TaskSummary) TaskSummary {
	return TaskSummary{
		ID:               task.ID,
		Question:         task.Question,
		Origin:           task.Origin,
		Options:          task.Options,
		VoiceInstruction: task.VoiceInstruction,
		OutcomeID:        task.OutcomeID,
		OutcomeName:      task.OutcomeName,
		CompetencyName:   task.CompetencyName,
		ConstituentName:  task.ConstituentName,
		TaxonomyCode:     task.TaxonomyCode,
		ALDLevelCode:     task.ALDLevelCode,
		TopicLevelCode:   task.TopicLevelCode,
		Importance:       task.Importance,
		IncludeInTest:    task.IncludeInTest,
	}
}

func taskProfileDTO(task application.TaskProfile) TaskProfile {
	sections := make([]CurriculumSection, len(task.CurriculumSections))
	for i, section := range task.CurriculumSections {
		sections[i] = CurriculumSection{Code: section.Code, Title: section.Title, CurriculumCompetencies: section.CurriculumCompetencies}
	}
	return TaskProfile{
		ID:                 task.ID,
		Question:           task.Question,
		Origin:             task.Origin,
		Options:            task.Options,
		VoiceInstruction:   task.VoiceInstruction,
		OutcomeID:          task.OutcomeID,
		OutcomeName:        task.OutcomeName,
		IncludeInTest:      task.IncludeInTest,
		TaxonomyCode:       task.TaxonomyCode,
		ALDLevelCode:       task.ALDLevelCode,
		Importance:         task.Importance,
		EducationalContent: task.EducationalContent,
		ConstituentID:      task.ConstituentID,
		ConstituentName:    task.ConstituentName,
		TopicLevelCode:     task.TopicLevelCode,
		CompetencyID:       task.CompetencyID,
		CompetencyName:     task.CompetencyName,
		CurriculumSections: sections,
	}
}
