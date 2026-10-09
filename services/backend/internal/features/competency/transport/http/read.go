package competencyhttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type MapSnapshot struct {
	Revision     int64        `json:"revision"`
	ImportedAt   *int64       `json:"imported_at" extensions:"x-nullable"`
	Competencies []Competency `json:"competencies"`
}

type Competency struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Constituents []Constituent `json:"constituents"`
}

type Constituent struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	TopicLevelCode     *string             `json:"topic_level_code" extensions:"x-nullable"`
	CurriculumSections []CurriculumSection `json:"curriculum_sections"`
	Outcomes           []Outcome           `json:"outcomes"`
}

type CurriculumSection struct {
	Code                   string   `json:"code"`
	Title                  string   `json:"title"`
	CurriculumCompetencies []string `json:"curriculum_competencies"`
}

type Outcome struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	IncludeInTest      *bool   `json:"include_in_test" extensions:"x-nullable"`
	TaxonomyCode       *string `json:"taxonomy_code" extensions:"x-nullable"`
	ALDLevelCode       *string `json:"ald_level_code" extensions:"x-nullable"`
	Importance         *int16  `json:"importance" extensions:"x-nullable"`
	EducationalContent *string `json:"educational_content" extensions:"x-nullable"`
	Tasks              []Task  `json:"tasks"`
}

type Task struct {
	ID               string   `json:"id"`
	Question         string   `json:"question"`
	Origin           string   `json:"origin"`
	Options          []string `json:"options"`
	VoiceInstruction *string  `json:"voice_instruction" extensions:"x-nullable"`
}

// ReadSubject returns the current map for one subject.
// @Summary Прочитать карту предмета
// @Description Ревизия, время импорта и дерево компетенций предмета. Эталоны и критерии не возвращаются.
// @ID readSubjectCompetencyMap
// @Tags Учебная база
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Success 200 {object} MapSnapshot
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /subjects/{subject_id}/competency-map [get]
func (h *Handler) ReadSubject(w http.ResponseWriter, r *http.Request) {
	h.readSubject(w, r, r.PathValue("subject_id"))
}

func (h *Handler) readSubject(w http.ResponseWriter, r *http.Request, subjectID string) {
	snapshot, err := h.service.ReadSubject(r.Context(), subjectID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, snapshotDTO(snapshot))
}

func snapshotDTO(snapshot competencymap.Snapshot) MapSnapshot {
	result := MapSnapshot{Revision: snapshot.Revision, ImportedAt: snapshot.ImportedAt, Competencies: make([]Competency, len(snapshot.Competencies))}
	for i, competency := range snapshot.Competencies {
		parent := Competency{ID: competency.ID, Name: competency.Name, Constituents: make([]Constituent, len(competency.Constituents))}
		for j, constituent := range competency.Constituents {
			child := Constituent{ID: constituent.ID, Name: constituent.Name, TopicLevelCode: constituent.TopicLevelCode,
				CurriculumSections: make([]CurriculumSection, len(constituent.Sections)), Outcomes: make([]Outcome, len(constituent.Outcomes))}
			for k, section := range constituent.Sections {
				child.CurriculumSections[k] = CurriculumSection{Code: section.Code, Title: section.Title, CurriculumCompetencies: section.CompetencyCodes}
			}
			for k, outcome := range constituent.Outcomes {
				item := Outcome{ID: outcome.ID, Name: outcome.Name, IncludeInTest: outcome.IncludeInTest,
					TaxonomyCode: outcome.TaxonomyCode, ALDLevelCode: outcome.ALDLevelCode, Importance: outcome.Importance,
					EducationalContent: outcome.EducationalContent, Tasks: make([]Task, len(outcome.Tasks))}
				for n, task := range outcome.Tasks {
					item.Tasks[n] = Task{ID: task.ID, Question: task.Question, Origin: task.Origin, Options: task.Options, VoiceInstruction: task.VoiceInstruction}
				}
				child.Outcomes[k] = item
			}
			parent.Constituents[j] = child
		}
		result.Competencies[i] = parent
	}
	return result
}
