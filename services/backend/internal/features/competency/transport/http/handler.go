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
