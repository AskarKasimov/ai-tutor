package subjecthttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/subject"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

type CreateRequest struct {
	Name string `json:"name" binding:"required" minLength:"1" maxLength:"200"`
}

// Create handles POST /admin/subjects.
// @Summary Создать предмет
// @Description Создать предмет без карты компетенций. Доступно только admin.
// @ID createSubject
// @Tags Предметы
// @Security accessCookie
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Название предмета"
// @Success 201 {object} subject.Subject
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /exam/admin/subjects [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	var request CreateRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	created, err := h.service.Create(r.Context(), actor, request.Name)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// CreateLearning is a compatibility alias for common course creation.
// @Summary Создать общий предмет
// @Description Совместимый адрес создания общей сущности курса. Новый интерфейс использует POST /exam/admin/subjects.
// @ID createLearningSubject
// @Tags Предметы обучения
// @Security accessCookie
// @Accept json
// @Produce json
// @Param request body CreateRequest true "Название предмета"
// @Success 201 {object} subject.Subject
// @Failure 401 {object} fault.Error
// @Failure 403 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /learn/admin/subjects [post]
func (h *Handler) CreateLearning(w http.ResponseWriter, r *http.Request) { h.Create(w, r) }

// List handles GET /subjects.
// @Summary Прочитать каталог предметов
// @Description Студентам возвращаются только готовые курсы: с фикстурой графа — сразу после создания, без неё — с опубликованным графом курса. Admin видит все предметы и признак готовности.
// @ID listSubjects
// @Tags Предметы
// @Security accessCookie
// @Produce json
// @Success 200 {array} subject.Subject
// @Failure 401 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /exam/subjects [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, false)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, learning bool) {
	actor, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	var items []subject.Subject
	var err error
	if learning {
		items, err = h.service.LearningCatalog(r.Context(), actor)
	} else {
		items, err = h.service.Catalog(r.Context(), actor)
	}
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if items == nil {
		items = []subject.Subject{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

// ListLearning returns common courses with learning-specific readiness.
// @Summary Прочитать каталог обучения
// @Description Прочитать каталог предметов режима обучения.
// @ID listLearningSubjects
// @Tags Предметы обучения
// @Security accessCookie
// @Produce json
// @Success 200 {array} subject.Subject
// @Failure 401 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /learn/subjects [get]
func (h *Handler) ListLearning(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, true)
}
