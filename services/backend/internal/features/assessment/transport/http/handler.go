package assessmenthttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type EvaluateRequest struct {
	TranscriptionID string `json:"transcription_id" binding:"required"`
	TaskID          string `json:"task_id" binding:"required"`
}

type EvaluateResponse struct {
	Score            int                           `json:"score" minimum:"0" maximum:"2"`
	Verdict          string                        `json:"verdict" enums:"correct,partial,incorrect"`
	CriterionResults []application.CriterionResult `json:"criterion_results"`
	Feedback         []string                      `json:"feedback" minItems:"3" maxItems:"3"`
}

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

// Evaluate handles POST /assessments/evaluate.
// @Summary Оценить сохранённый голосовой ответ
// @Description Принимает ID задания и ID принадлежащей студенту расшифровки. Контекст задания берётся сервером.
// @ID evaluateAnswer
// @Tags Грейдинг и фидбэк
// @Security accessCookie
// @Accept json
// @Produce json
// @Param request body EvaluateRequest true "Задание и расшифровка"
// @Success 200 {object} EvaluateResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /assessments/evaluate [post]
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	u, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var req EvaluateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Evaluate(r.Context(), u.ID, req.TranscriptionID, req.TaskID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, EvaluateResponse{
		Score:            result.Score,
		Verdict:          result.Verdict,
		CriterionResults: result.CriterionResults,
		Feedback:         result.Feedback,
	})
}
