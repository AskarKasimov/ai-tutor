package taskgenhttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type GenerateRequest struct {
	Count int `json:"count,omitempty" binding:"optional" minimum:"1" maximum:"5"`
}

type Outcome struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ConstituentName string `json:"constituent_name"`
	CompetencyName  string `json:"competency_name"`
	TaskCount       int    `json:"task_count" minimum:"0"`
}

type OutcomeList struct {
	Outcomes []Outcome `json:"outcomes"`
}

type Task struct {
	Question         string   `json:"question"`
	Criteria         string   `json:"criteria"`
	VoiceInstruction string   `json:"voice_instruction"`
	Options          []string `json:"options"`
}

type GenerateResponse struct {
	OutcomeID   string `json:"outcome_id"`
	OutcomeName string `json:"outcome_name"`
	Tasks       []Task `json:"tasks"`
}

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

// ListOutcomes handles GET /outcomes.
// @Summary Темы (ОР) карты компетенций для тренировки
// @Description Возвращает опубликованные в текущей карте темы с числом готовых заданий. Доступно авторизованному пользователю.
// @ID listTrainingOutcomes
// @Tags Генерация заданий
// @Security accessCookie
// @Produce json
// @Success 200 {object} OutcomeList
// @Failure 401 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /outcomes [get]
func (h *Handler) ListOutcomes(w http.ResponseWriter, r *http.Request) {
	if _, ok := httpx.Principal[user.User](r); !ok {
		httpx.Error(w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	outcomes, err := h.service.Outcomes(r.Context())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	result := OutcomeList{Outcomes: make([]Outcome, 0, len(outcomes))}
	for _, outcome := range outcomes {
		result.Outcomes = append(result.Outcomes, Outcome{ID: outcome.ID, Name: outcome.Name, ConstituentName: outcome.ConstituentName, CompetencyName: outcome.CompetencyName, TaskCount: outcome.TaskCount})
	}
	httpx.JSON(w, http.StatusOK, result)
}

// GenerateTrainingTasks handles POST /outcomes/{outcomeId}/training-tasks.
// @Summary Сгенерировать тренировочные задания по аналогии с заданиями темы
// @Description Доступно авторизованному пользователю. Берёт задания выбранного ОР как образец и просит LLM построить новые аналогичные задания. Результат не сохраняется.
// @ID generateTrainingTasks
// @Tags Генерация заданий
// @Security accessCookie
// @Accept json
// @Produce json
// @Param outcomeId path string true "Идентификатор темы (ОР)"
// @Param request body GenerateRequest true "Число заданий"
// @Success 200 {object} GenerateResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /outcomes/{outcomeId}/training-tasks [post]
func (h *Handler) GenerateTrainingTasks(w http.ResponseWriter, r *http.Request) {
	if _, ok := httpx.Principal[user.User](r); !ok {
		httpx.Error(w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	var req GenerateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	result, err := h.service.Generate(r.Context(), r.PathValue("outcomeId"), req.Count)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	response := GenerateResponse{OutcomeID: result.OutcomeID, OutcomeName: result.OutcomeName, Tasks: make([]Task, 0, len(result.Tasks))}
	for _, task := range result.Tasks {
		options := task.Options
		if options == nil {
			options = []string{}
		}
		response.Tasks = append(response.Tasks, Task{Question: task.Question, Criteria: task.Criteria, VoiceInstruction: task.VoiceInstruction, Options: options})
	}
	httpx.JSON(w, http.StatusOK, response)
}
