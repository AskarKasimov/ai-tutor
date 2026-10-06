package taskgenhttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handlers struct {
	service   *application.Service
	materials *application.MaterialService
}

func New(service *application.Service, materials *application.MaterialService) *Handlers {
	return &Handlers{service: service, materials: materials}
}

type GenerateRequest struct {
	OutcomeID string `json:"outcome_id" minLength:"1" maxLength:"128" binding:"required"`
}

type GeneratedTask struct {
	ID               string   `json:"id"`
	OutcomeID        string   `json:"outcome_id"`
	Question         string   `json:"question"`
	Options          []string `json:"options"`
	VoiceInstruction *string  `json:"voice_instruction" extensions:"x-nullable"`
	Origin           string   `json:"origin"`
}

// Generate creates and immediately publishes one task for an outcome.
// @Summary Сгенерировать аналог задания
// @ID generateTask
// @Tags Банк заданий
// @Security accessCookie
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param request body GenerateRequest true "Образовательный результат"
// @Success 201 {object} GeneratedTask
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /tasks/generate [post]
func (h *Handlers) Generate(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	var request GenerateRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpx.Error(r.Context(), w, fault.Validation("Idempotency-Key", "Укажите ключ идемпотентности."))
		return
	}
	task, err := h.service.Generate(r.Context(), request.OutcomeID, key, principal.ID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, GeneratedTask{
		ID: task.ID, OutcomeID: task.OutcomeID, Question: task.Question,
		Options: task.Options, VoiceInstruction: task.VoiceInstruction, Origin: task.Origin,
	})
}

type ImportMaterialRequest struct {
	Name       string   `json:"name"`
	Content    string   `json:"content"`
	OutcomeIDs []string `json:"outcome_ids"`
}

// ImportMaterial adds text chunks and links them to outcomes for retrieval.
// @Summary Загрузить учебный материал в контекст RAG
// @ID importMaterial
// @Tags Учебная база
// @Security accessCookie
// @Accept json
// @Produce json
// @Param request body ImportMaterialRequest true "Текст и связи материала"
// @Success 201 {object} map[string]string
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /admin/materials [post]
func (h *Handlers) ImportMaterial(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok || principal.Role != user.Admin {
		httpx.Error(r.Context(), w, fault.New(fault.Forbidden, "FORBIDDEN", "Недостаточно прав."))
		return
	}
	var request ImportMaterialRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if err := h.materials.Import(r.Context(), application.MaterialInput{Name: request.Name, Content: request.Content, OutcomeIDs: request.OutcomeIDs}); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "imported"})
}
