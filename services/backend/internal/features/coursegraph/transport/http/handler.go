package http

import (
	"net/http"

	_ "github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// MaxMapBytes bounds an imported competency map book.
const MaxMapBytes = 25 << 20

type Handler struct{ service *application.Worker }

func New(service *application.Worker) *Handler { return &Handler{service} }

// Read returns the latest successful graph and current build status.
// @Summary Граф курса и состояние построения
// @Description Доступно только admin. Граф собирается из материалов кнопкой POST .../graph/build. graph_source — built (собран из материалов), competency_map (историческая запись) или none. Новая экзаменационная сборка доступна только при готовых материалах; прежние map snapshots сохраняются для совместимости чтения.
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
// @Router /exam/admin/subjects/{subject_id}/graph [get]
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

// Build starts a build of the course graph from the current materials.
// @Summary Собрать граф курса
// @Description Доступно только admin. Кнопка «Собрать граф»: ставит сборку из текущих материалов курса. Опубликованный граф остаётся у Geek до успешной сборки; успешная сборка с другим графом переводит активные диагностики курса в cancelled.
// @ID buildCourseGraph
// @Tags Материалы
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Success 202 {object} coursegraph.Snapshot
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error "GRAPH_BUILD_IN_PROGRESS: сборка уже идёт"
// @Failure 422 {object} fault.Error "GRAPH_INPUTS_NOT_READY: материалы ещё обрабатываются, не обработаны или их нет"
// @Failure 503 {object} fault.Error
// @Router /exam/admin/subjects/{subject_id}/graph/build [post]
func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется сессия."))
		return
	}
	result, err := h.service.Request(r.Context(), actor, r.PathValue("subject_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, result)
}
