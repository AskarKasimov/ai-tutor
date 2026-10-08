package variantgenhttp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

// Create creates and stores a variant from the active competency map.
// @Summary Собрать и сохранить вариант
// @Description Вариант собирается по всей карте без тела запроса. Для включения достаточно одного готового TRUE-ОР. Выбираются основной и до двух базовых ОР с рангом Блума ниже основного. Неуспешные компетенции возвращаются с причиной. Повтор успешного запроса с тем же ключом возвращает сохранённый вариант.
// @Tags Варианты
// @Security accessCookie
// @Param Idempotency-Key header string true "Ключ повтора успешного запроса" minlength(1) maxlength(128)
// @Success 201 {object} Variant
// @Failure 401 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /variants [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется авторизация."))
		return
	}
	if err := httpx.NoBody(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpx.Error(r.Context(), w, fault.Validation("Idempotency-Key", "Заголовок Idempotency-Key обязателен."))
		return
	}
	value, err := h.service.Create(r.Context(), principal.ID, key)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, variantDTO(value))
}

// List reads the authenticated user's saved variants.
// @Summary Прочитать свои сохранённые варианты
// @Description Возвращает только метаданные вариантов владельца с cursor-пагинацией по времени создания и ID. Query-параметры должны быть известными, непустыми и передаваться однократно. Некорректное кодирование и недопустимые значения возвращают 422.
// @Tags Варианты
// @Security accessCookie
// @Param limit query int false "Размер страницы, 1–100" default(20) minimum(1) maximum(100)
// @Param cursor query string false "Курсор страницы"
// @Success 200 {object} VariantList
// @Failure 401 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /variants [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется авторизация."))
		return
	}
	query, err := httpx.ParseQuery(r, "limit", "cursor")
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			httpx.Error(r.Context(), w, fault.Validation("limit", "Лимит должен быть от 1 до 100."))
			return
		}
		limit = parsed
	}
	items, cursor, err := h.service.List(r.Context(), principal.ID, limit, query.Get("cursor"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, listDTO(items, cursor))
}

// Read returns the saved variant plan, including main and both basic tasks.
// @Summary Прочитать вариант
// @Description Возвращает сохранённый план всей включённой карты, включая тексты main и обоих basic. Эталоны, criteria и ОС не выдаются.
// @Tags Варианты
// @Security accessCookie
// @Param id path string true "ID варианта" minlength(1) maxlength(128)
// @Success 200 {object} Variant
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /variants/{id} [get]
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется авторизация."))
		return
	}
	value, err := h.service.Get(r.Context(), principal.ID, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, variantDTO(value))
}

// ReadTask returns one saved public task position.
// @Summary Прочитать задание варианта
// @Description Возвращает публичные поля задания и профиля без эталона, criteria и ОС. Позиция должна принадлежать указанному варианту и владельцу.
// @Tags Варианты
// @Security accessCookie
// @Param id path string true "ID варианта" minlength(1) maxlength(128)
// @Param task_id path string true "ID позиции варианта" minlength(1) maxlength(128)
// @Success 200 {object} VariantTask
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /variants/{id}/tasks/{task_id} [get]
func (h *Handler) ReadTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется авторизация."))
		return
	}
	value, err := h.service.Task(r.Context(), principal.ID, strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("task_id")))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, taskDTO(value))
}
