// Package learninghttp exposes the personal learning plan.
package learninghttp

import (
	"net/http"
	"strconv"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/learning"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handler struct {
	service  *application.Service
	maxBytes int64
}

func New(service *application.Service, maxBytes int64) *Handler {
	return &Handler{service: service, maxBytes: maxBytes}
}

type CreatePlanRequest struct {
	Hours int `json:"hours" minimum:"1" maximum:"100" binding:"required"`
}

type PlanItemResponse struct {
	Index           int    `json:"index"`
	CompetencyID    string `json:"competency_id"`
	CompetencyName  string `json:"competency_name"`
	OutcomeID       string `json:"outcome_id"`
	OutcomeName     string `json:"outcome_name"`
	TheoryMinutes   int    `json:"theory_minutes"`
	PracticeMinutes int    `json:"practice_minutes"`
	InPlan          bool   `json:"in_plan"`
	Status          string `json:"status" enums:"pending,current,passed,review"`
	Attempts        int    `json:"attempts"`
}

type PlanResponse struct {
	ID                  string             `json:"id"`
	DiagnosticSessionID string             `json:"diagnostic_session_id"`
	SubjectID           string             `json:"subject_id"`
	SubjectName         string             `json:"subject_name"`
	Hours               int                `json:"hours"`
	Detail              string             `json:"detail" enums:"brief,standard,deep"`
	Status              string             `json:"status" enums:"active,completed"`
	CurrentIndex        *int               `json:"current_index" extensions:"x-nullable"`
	ProgressPercent     int                `json:"progress_percent"`
	PlannedMinutes      int                `json:"planned_minutes"`
	PendingAnswer       *string            `json:"pending_answer" extensions:"x-nullable"`
	Items               []PlanItemResponse `json:"items"`
}

type MessageResponse struct {
	Sequence  int64  `json:"sequence"`
	ItemIndex int    `json:"item_index"`
	Role      string `json:"role" enums:"assistant,user"`
	Kind      string `json:"kind" enums:"theory,practice,answer,feedback"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

type MessagesResponse struct {
	Items []MessageResponse `json:"items"`
}

func planDTO(plan learning.Plan) PlanResponse {
	response := PlanResponse{
		ID: plan.ID, DiagnosticSessionID: plan.DiagnosticID, SubjectID: plan.SubjectID, SubjectName: plan.SubjectName,
		Hours: plan.Hours, Detail: string(plan.Detail), Status: "active", ProgressPercent: plan.Progress(),
		PendingAnswer: plan.PendingAnswer, Items: make([]PlanItemResponse, 0, len(plan.Items)),
	}
	if plan.Completed() {
		response.Status = "completed"
	} else {
		current := plan.Current
		response.CurrentIndex = &current
	}
	for i, item := range plan.Items {
		if item.InPlan {
			response.PlannedMinutes += item.Minutes()
		}
		response.Items = append(response.Items, PlanItemResponse{
			Index: i, CompetencyID: item.CompetencyID, CompetencyName: item.CompetencyName,
			OutcomeID: item.OutcomeID, OutcomeName: item.OutcomeName,
			TheoryMinutes: item.TheoryMinutes, PracticeMinutes: item.PracticeMinutes,
			InPlan: item.InPlan, Status: string(item.Status), Attempts: item.Attempts,
		})
	}
	return response
}

func owner(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется авторизация."))
	}
	return p.ID, ok
}

func reply(w http.ResponseWriter, r *http.Request, status int, plan learning.Plan, err error) {
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, status, planDTO(plan))
}

// Create builds the learning plan of a completed diagnostic.
// @Summary Создать план обучения
// @Description План строится один раз: в него входят все ОР компетенций с ошибками, важные первыми, пока хватает часов; остальные — «если останется время». Повторный запрос возвращает уже созданный план.
// @Tags Обучение
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID диагностики"
// @Param request body CreatePlanRequest true "Часы на изучение предмета"
// @Success 200 {object} PlanResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions/{id}/learning-plan [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	var input CreatePlanRequest
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	plan, err := h.service.Create(r.Context(), id, r.PathValue("id"), input.Hours)
	reply(w, r, 200, plan, err)
}

// ByDiagnostic returns the learning plan of a diagnostic.
// @Summary План обучения по диагностике
// @Tags Обучение
// @Security accessCookie
// @Produce json
// @Param id path string true "ID диагностики"
// @Success 200 {object} PlanResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /diagnostic-sessions/{id}/learning-plan [get]
func (h *Handler) ByDiagnostic(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	plan, err := h.service.ByDiagnostic(r.Context(), id, r.PathValue("id"))
	reply(w, r, 200, plan, err)
}

// Get returns a learning plan.
// @Summary План обучения
// @Tags Обучение
// @Security accessCookie
// @Produce json
// @Param id path string true "ID плана"
// @Success 200 {object} PlanResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /learning-plans/{id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	plan, err := h.service.Get(r.Context(), id, r.PathValue("id"))
	reply(w, r, 200, plan, err)
}

// Messages returns the lesson chat of a plan item.
// @Summary Чат урока по теме плана
// @Tags Обучение
// @Security accessCookie
// @Produce json
// @Param id path string true "ID плана"
// @Param index path int true "Номер темы плана"
// @Success 200 {object} MessagesResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /learning-plans/{id}/items/{index}/messages [get]
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		httpx.Error(r.Context(), w, fault.New(fault.NotFound, "LEARNING_ITEM_NOT_FOUND", "Тема плана не найдена."))
		return
	}
	messages, err := h.service.Messages(r.Context(), id, r.PathValue("id"), index)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	response := MessagesResponse{Items: make([]MessageResponse, 0, len(messages))}
	for _, m := range messages {
		response.Items = append(response.Items, MessageResponse{Sequence: m.Sequence, ItemIndex: m.ItemIndex, Role: string(m.Role), Kind: string(m.Kind), Text: m.Text, CreatedAt: m.CreatedAt})
	}
	httpx.JSON(w, 200, response)
}

// Open saves theory first, then prepares and reviews the practice question.
// @Summary Открыть урок текущей темы
// @Description Теория сохраняется сразу и доступна через сообщения темы, пока готовится практика. Ответ возвращается после готовности вопроса. При ошибке практики теория сохраняется; повторный запрос продолжает с неё. Готовый урок не пересоздаётся.
// @Tags Обучение
// @Security accessCookie
// @Produce json
// @Param id path string true "ID плана"
// @Success 200 {object} PlanResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /learning-plans/{id}/open [post]
func (h *Handler) Open(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	plan, err := h.service.Open(r.Context(), id, r.PathValue("id"))
	reply(w, r, 200, plan, err)
}

// Answer accepts a spoken answer to the practice question of the current topic.
// @Summary Ответить голосом на практику
// @Description Расшифровка сохраняется сразу, затем наставник проверяет ответ. Верный ответ завершает тему; неверный получает разбор и новый вопрос, после трёх попыток тема помечается «повторить».
// @Tags Обучение
// @Security accessCookie
// @Accept mpfd
// @Produce json
// @Param id path string true "ID плана"
// @Param audio formData file true "Голосовой ответ"
// @Success 200 {object} PlanResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 415 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /learning-plans/{id}/answers [post]
func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	data, media, err := httpx.Upload(r, "audio", h.maxBytes)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	plan, err := h.service.Answer(r.Context(), id, r.PathValue("id"), data, media)
	reply(w, r, 200, plan, err)
}

// Retry checks the saved answer again after the check failed.
// @Summary Повторить проверку ответа
// @Tags Обучение
// @Security accessCookie
// @Produce json
// @Param id path string true "ID плана"
// @Success 200 {object} PlanResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /learning-plans/{id}/answers/retry [post]
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	plan, err := h.service.Retry(r.Context(), id, r.PathValue("id"))
	reply(w, r, 200, plan, err)
}
