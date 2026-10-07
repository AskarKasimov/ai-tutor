package http

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Handler struct {
	service *application.Service
}

func New(service *application.Service) *Handler {
	return &Handler{service: service}
}

// GetFeedback generates or reads cached overall diagnostic feedback.
// @Summary Получить итоговый педагогический фидбэк по диагностической сессии
// @Description Детерминированно агрегирует результаты сессии (освоенные темы, частичные знания, подтверждённые пробелы) и формирует педагогическое резюме.
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param id path string true "ID сессии"
// @Success 200 {object} OverallFeedbackResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/feedback [get]
func (h *Handler) GetFeedback(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if r.URL.RawQuery != "" {
		httpx.Error(r.Context(), w, fault.Validation("query", "Параметры запроса не поддерживаются."))
		return
	}
	sessionID := r.PathValue("id")
	feedback, err := h.service.GetFeedback(r.Context(), principal.ID, sessionID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, feedbackResponse(feedback))
}

// SynthesizeAnswersFeedback handles POST /assessments/overall-feedback.
// @Summary Сгенерировать итоговый педагогический фидбэк по ответам сессии
// @Description Принимает список выполненных заданий сессии с частными оценками и замечаниями и возвращает единый синтезированный фидбэк.
// @Tags Грейдинг и фидбэк
// @Security accessCookie
// @Accept json
// @Produce json
// @Param request body AnswersFeedbackRequest true "Список выполненных заданий с частными оценками"
// @Success 200 {object} AnswersFeedbackResponse
// @Failure 401 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /assessments/overall-feedback [post]
func (h *Handler) SynthesizeAnswersFeedback(w http.ResponseWriter, r *http.Request) {
	_, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var req AnswersFeedbackRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if len(req.Answers) == 0 {
		httpx.Error(r.Context(), w, fault.Validation("answers", "Список ответов не может быть пустым."))
		return
	}

	items := make([]application.AnswerFeedbackItem, 0, len(req.Answers))
	for _, a := range req.Answers {
		items = append(items, application.AnswerFeedbackItem{
			TaskID:     a.TaskID,
			Topic:      a.Topic,
			Question:   a.Question,
			Transcript: a.Transcript,
			Score:      a.Score,
			MaxScore:   a.MaxScore,
			Feedback:   append([]string(nil), a.Feedback...),
		})
	}

	res, err := h.service.GenerateAnswersFeedback(r.Context(), items)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, AnswersFeedbackResponse{
		Score:           res.Score,
		MaxScore:        res.MaxScore,
		ScorePercentage: res.ScorePercentage,
		Summary:         res.Summary,
		Strengths:       res.Strengths,
		Gaps:            res.Gaps,
		Partials:        res.Partials,
		Recommendations: res.Recommendations,
		GeneratedAt:     res.GeneratedAt,
	})
}
