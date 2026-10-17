package http

import (
	_ "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"net/http"
)

type Handler struct{ service *application.Worker }

func New(service *application.Worker) *Handler { return &Handler{service} }

// Read returns the latest successful graph and current build status.
// @Summary Граф курса и состояние построения
// @Description Доступно только admin. Автоматическая сборка начинается после обработки всех материалов. stale означает, что материалы изменились или ещё обрабатываются.
// @ID getCourseGraph
// @Tags Материалы
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Success 200 {object} coursegraph.Snapshot
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /admin/subjects/{subject_id}/graph [get]
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется сессия."))
		return
	}
	result, err := h.service.Read(r.Context(), actor, r.PathValue("subject_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}
