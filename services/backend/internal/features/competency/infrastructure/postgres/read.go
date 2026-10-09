package postgres

import (
	"context"
	"encoding/json"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func (r *Repository) Read(ctx context.Context, subjectID string) (competencymap.Snapshot, error) {
	// One SQL statement observes one revision and its entire graph, even during import.
	rows, err := r.queries.ReadSubjectCompetencyMap(ctx, subjectID)
	if err != nil {
		return competencymap.Snapshot{}, err
	}
	if len(rows) == 0 {
		return competencymap.Snapshot{}, fault.New(fault.NotFound, "SUBJECT_NOT_FOUND", "Предмет не найден.")
	}
	result := competencymap.Snapshot{Competencies: []competencymap.CompetencyNode{}}
	if len(rows) > 0 {
		result.Revision, result.ImportedAt = rows[0].Revision, rows[0].ImportedAt
	}
	// The query groups each parent's children contiguously in deterministic order.
	for _, row := range rows {
		if row.CompetencyID == nil {
			continue
		}
		if len(result.Competencies) == 0 || result.Competencies[len(result.Competencies)-1].ID != *row.CompetencyID {
			result.Competencies = append(result.Competencies, competencymap.CompetencyNode{
				ID: *row.CompetencyID, Name: *row.CompetencyName, Constituents: []competencymap.ConstituentNode{},
			})
		}
		competency := &result.Competencies[len(result.Competencies)-1]
		if row.ConstituentID == nil {
			continue
		}
		if len(competency.Constituents) == 0 || competency.Constituents[len(competency.Constituents)-1].ID != *row.ConstituentID {
			var storedSections []struct {
				Code, Title     string
				CompetencyCodes []string `json:"curriculum_competencies"`
			}
			if err := json.Unmarshal([]byte(row.CurriculumSections), &storedSections); err != nil {
				return competencymap.Snapshot{}, err
			}
			sections := make([]competencymap.CurriculumSection, len(storedSections))
			for i, section := range storedSections {
				sections[i] = competencymap.CurriculumSection{Code: section.Code, Title: section.Title, CompetencyCodes: section.CompetencyCodes}
			}
			competency.Constituents = append(competency.Constituents, competencymap.ConstituentNode{
				ID: *row.ConstituentID, Name: *row.ConstituentName, TopicLevelCode: row.TopicLevelCode,
				Sections: sections, Outcomes: []competencymap.OutcomeNode{},
			})
		}
		constituent := &competency.Constituents[len(competency.Constituents)-1]
		if row.OutcomeID == nil {
			continue
		}
		if len(constituent.Outcomes) == 0 || constituent.Outcomes[len(constituent.Outcomes)-1].ID != *row.OutcomeID {
			constituent.Outcomes = append(constituent.Outcomes, competencymap.OutcomeNode{
				ID: *row.OutcomeID, Name: *row.OutcomeName, IncludeInTest: row.IncludeInTest,
				TaxonomyCode: row.TaxonomyCode, ALDLevelCode: row.AldLevelCode, Importance: row.Importance,
				EducationalContent: row.EducationalContent, Tasks: []competencymap.TaskNode{},
			})
		}
		if row.TaskID == nil {
			continue
		}
		var options []string
		if err := json.Unmarshal(row.Options, &options); err != nil {
			return competencymap.Snapshot{}, err
		}
		if options == nil {
			options = []string{}
		}
		outcome := &constituent.Outcomes[len(constituent.Outcomes)-1]
		outcome.Tasks = append(outcome.Tasks, competencymap.TaskNode{
			ID: *row.TaskID, Question: *row.Question, Origin: *row.Origin,
			Options: options, VoiceInstruction: row.VoiceInstruction,
		})
	}
	return result, nil
}
