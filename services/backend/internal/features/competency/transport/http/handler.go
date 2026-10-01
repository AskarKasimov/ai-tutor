package competencyhttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type ImportResult struct {
	Revision         int64 `json:"revision"`
	ImportedAt       int64 `json:"imported_at"`
	CompetencyCount  int   `json:"competency_count"`
	ConstituentCount int   `json:"constituent_count"`
	OutcomeCount     int   `json:"outcome_count"`
	TaskCount        int   `json:"task_count"`
}

type Handler struct {
	service        *application.Service
	maxUploadBytes int64
}

func New(service *application.Service, maxUploadBytes int64) *Handler {
	return &Handler{service: service, maxUploadBytes: maxUploadBytes}
}

// Import handles POST /admin/competency-map/import.
// @Summary Заменить учебную базу картой компетенций из CSV
// @Description Доступно только admin. Импорт атомарно заменяет учебную базу.
// @ID importCompetencyMap
// @Tags Учебная база
// @Security accessCookie
// @Accept mpfd
// @Produce json
// @Param file formData file true "CSV карты компетенций, UTF-8, до 25 МиБ"
// @Success 200 {object} ImportResult
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 415 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /admin/competency-map/import [post]
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := h.service.Authorize(actor); err != nil {
		httpx.Error(w, err)
		return
	}
	data, media, err := httpx.Upload(r, "file", h.maxUploadBytes)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	result, err := h.service.Import(r.Context(), actor, data, media)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ImportResult{
		Revision: result.Revision, ImportedAt: result.ImportedAt,
		CompetencyCount: result.CompetencyCount, ConstituentCount: result.ConstituentCount,
		OutcomeCount: result.OutcomeCount, TaskCount: result.TaskCount,
	})
}
