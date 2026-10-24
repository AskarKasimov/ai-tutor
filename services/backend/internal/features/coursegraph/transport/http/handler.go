package http

import (
	"errors"
	"io"
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
// @Description Доступно только admin. Сборку запускает POST .../graph/build. stale — источник графа (материалы или карта) изменился после публикации; inputs_ready — все материалы обработаны и сборку можно запустить. graph_source — откуда граф: built (собран из материалов), competency_map (собран по карте компетенций) или none — графа нет, курс студентам не виден. build_source — что читает последняя запрошенная сборка; map — последняя импортированная карта.
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

// ImportMap builds the course graph from a competency map.
// @Summary Импортировать карту компетенций
// @Description Доступно только admin. Кнопка «Импортировать карту компетенций»: граф курса собирается по карте вместо материалов — на каждую субкомпетенцию с тремя и более ОР с TRUE и заданием основное задание (наивысший Блум, затем важность) становится темой верхнего уровня, два самых простых — её пререквизитами; вопрос — задание карты без изменений. Карта проверяется сразу, затем вопросы готовятся и граф публикуется так же, как сборка из материалов, и заменяет прежний граф курса. Граф строится по первому листу книги (второй — для диагностики режима «Обучение»); колонки «Компетенция» и «Таксономия» принимаются вместо «Субкомпетенция» и «Уровень по Блуму». Все колонки «Задание N» ОР становятся вариантами темы с критериями из колонки «Критерии оценивания…» того же номера по порядку; экзамен при старте выбирает один вариант случайно.
// @ID importCourseGraphCompetencyMap
// @Tags Материалы
// @Security accessCookie
// @Accept multipart/form-data
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Param file formData file true "Карта компетенций .xlsx, до 25 МиБ"
// @Success 202 {object} coursegraph.Snapshot
// @Failure 400 {object} fault.Error
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error "GRAPH_BUILD_IN_PROGRESS: сборка уже идёт"
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error "COMPETENCY_MAP_INVALID или COMPETENCY_MAP_EMPTY"
// @Failure 503 {object} fault.Error
// @Router /exam/admin/subjects/{subject_id}/graph/competency-map [post]
func (h *Handler) ImportMap(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется сессия."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxMapBytes+1<<20)
	if err := r.ParseMultipartForm(MaxMapBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "PAYLOAD_TOO_LARGE", "Файл карты больше 25 МиБ."))
			return
		}
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Ожидается один файл карты .xlsx."))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		httpx.Error(r.Context(), w, fault.Validation("file", "Нужно выбрать ровно один файл карты .xlsx."))
		return
	}
	if files[0].Size > MaxMapBytes {
		httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "PAYLOAD_TOO_LARGE", "Файл карты больше 25 МиБ."))
		return
	}
	f, err := files[0].Open()
	if err != nil {
		httpx.Error(r.Context(), w, fault.New(fault.Invalid, "BAD_REQUEST", "Не удалось прочитать файл карты."))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxMapBytes+1))
	if err != nil || len(data) > MaxMapBytes {
		httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "PAYLOAD_TOO_LARGE", "Файл карты больше 25 МиБ."))
		return
	}
	result, err := h.service.ImportMap(r.Context(), actor, r.PathValue("subject_id"), files[0].Filename, data)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, result)
}
