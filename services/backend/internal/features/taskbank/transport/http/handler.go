package taskbankhttp

import (
	"net/http"
	"strconv"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

// Search filters student-visible tasks using their competency profile.
// @Summary Найти задания по свойствам карты
// @ID searchTasks
// @Tags Банк заданий
// @Security accessCookie
// @Produce json
// @Param outcome_id query string false "ID образовательного результата"
// @Param competency_id query string false "ID компетенции"
// @Param constituent_id query string false "ID составляющей"
// @Param taxonomy query string false "Код таксономии"
// @Param ald_level query string false "Код уровня ALDs"
// @Param topic_level query string false "Код уровня темы"
// @Param importance query int false "Точная важность 1–5"
// @Param importance_min query int false "Минимальная важность 1–5 (включительно)"
// @Param importance_max query int false "Максимальная важность 1–5 (включительно)"
// @Param include_in_test query bool false "Фильтр включения в тест"
// @Param section query string false "Код раздела РПД"
// @Param curriculum_competency query string false "Код компетенции РПД"
// @Param origin query string false "authored или ai_generated"
// @Param limit query int false "Максимум 100"
// @Success 200 {array} application.TaskSummary
// @Failure 401 {object} fault.Error
// @Router /tasks [get]
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := application.SearchFilter{
		OutcomeID: query.Get("outcome_id"), CompetencyID: query.Get("competency_id"),
		ConstituentID: query.Get("constituent_id"), TaxonomyCode: query.Get("taxonomy"),
		ALDLevelCode: query.Get("ald_level"), TopicLevelCode: query.Get("topic_level"),
		Origin: query.Get("origin"), SectionCode: query.Get("section"),
		CurriculumCompetencyCode: query.Get("curriculum_competency"), Limit: 50,
	}
	if raw := query.Get("importance"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 1 || value > 5 {
			httpx.Error(r.Context(), w, fault.Validation("importance", "Важность должна быть от 1 до 5."))
			return
		}
		filter.Importance = int32(value)
	}
	for _, bound := range []struct {
		name string
		dst  *int32
	}{
		{"importance_min", &filter.ImportanceMin}, {"importance_max", &filter.ImportanceMax},
	} {
		if raw := query.Get(bound.name); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 32)
			if err != nil || value < 1 || value > 5 {
				httpx.Error(r.Context(), w, fault.Validation(bound.name, "Важность должна быть от 1 до 5."))
				return
			}
			*bound.dst = int32(value)
		}
	}
	if filter.ImportanceMin > 0 && filter.ImportanceMax > 0 && filter.ImportanceMin > filter.ImportanceMax {
		httpx.Error(r.Context(), w, fault.Validation("importance_min", "Минимальная важность не должна превышать максимальную."))
		return
	}
	if raw := query.Get("include_in_test"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.Error(r.Context(), w, fault.Validation("include_in_test", "Ожидается true или false."))
			return
		}
		filter.IncludeInTest = &value
	}
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 1 || value > 100 {
			httpx.Error(r.Context(), w, fault.Validation("limit", "Лимит должен быть от 1 до 100."))
			return
		}
		filter.Limit = int32(value)
	}
	result, err := h.service.Search(r.Context(), filter)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

// Profile returns task and competency mapping without its grading secrets.
// @Summary Получить профиль задания
// @ID getTaskProfile
// @Tags Банк заданий
// @Security accessCookie
// @Produce json
// @Param id path string true "ID задания"
// @Success 200 {object} application.TaskProfile
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /tasks/{id} [get]
func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.Profile(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}
