package variantgenhttp

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"net/http"
)

type DiagnosticOutline struct {
	Revision         int64               `json:"revision"`
	AlgorithmVersion string              `json:"algorithm_version"`
	Competencies     []OutlineCompetency `json:"competencies"`
}
type OutlineCompetency struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Constituents []OutlineConstituent `json:"constituents"`
}
type OutlineConstituent struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Outcomes []OutlineOutcome `json:"outcomes"`
}
type OutlineOutcome struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role" enums:"main,basic"`
}

// Preview returns only the blocks and outcomes selected for diagnostics.
// @Summary Предпросмотр блоков диагностики
// @Description Применяет отбор competency-map-v4 к текущей карте без сохранения варианта и запуска диагностики. Возвращает только названия и ID выбранных компетенций, составляющих и ОР с ролью main/basic. Вопросы, эталоны, criteria и ОС не выдаются. Пустой отбор возвращается пустым списком.
// @ID previewDiagnosticOutline
// @Tags Варианты
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Success 200 {object} DiagnosticOutline
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /learn/subjects/{subject_id}/diagnostic-outline [get]
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	value, err := h.service.Preview(r.Context(), r.PathValue("subject_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, outlineDTO(value))
}

func outlineDTO(value variant.Variant) DiagnosticOutline {
	result := DiagnosticOutline{Revision: value.MapRevision, AlgorithmVersion: value.AlgorithmVersion, Competencies: make([]OutlineCompetency, 0, len(value.Competencies))}
	for _, selection := range value.Competencies {
		competency := OutlineCompetency{ID: selection.Competency.ID, Name: selection.Competency.Name, Constituents: []OutlineConstituent{}}
		positions := map[string]int{}
		for _, task := range selection.Tasks {
			topic := task.Task.Constituent
			position, found := positions[topic.ID]
			if !found {
				position = len(competency.Constituents)
				positions[topic.ID] = position
				competency.Constituents = append(competency.Constituents, OutlineConstituent{ID: topic.ID, Name: topic.Name, Outcomes: []OutlineOutcome{}})
			}
			competency.Constituents[position].Outcomes = append(competency.Constituents[position].Outcomes, OutlineOutcome{ID: task.Task.Outcome.ID, Name: task.Task.Outcome.Name, Role: task.Role})
		}
		result.Competencies = append(result.Competencies, competency)
	}
	return result
}
