package assessmenthttp

import (
	"net/http"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/assessment"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type EvaluateRequest struct {
	TranscriptionID string `json:"transcription_id" minLength:"1" maxLength:"128" binding:"required"`
	VariantID       string `json:"variant_id" minLength:"1" maxLength:"128" binding:"required"`
	VariantTaskID   string `json:"variant_task_id" minLength:"1" maxLength:"128" binding:"required"`
}

type EvaluateResponse struct {
	Score            int                          `json:"score" minimum:"0" maximum:"2"`
	MaxScore         int                          `json:"max_score" minimum:"1" maximum:"2"`
	Verdict          string                       `json:"verdict" enums:"correct,partial,incorrect"`
	CriterionResults []assessment.CriterionResult `json:"criterion_results"`
	Feedback         []string                     `json:"feedback" minItems:"3" maxItems:"3"`
}

type Handler struct{ service *application.Service }

func New(service *application.Service) *Handler { return &Handler{service: service} }

// Evaluate handles POST /assessments/evaluate.
// @Summary Оценить сохранённый голосовой ответ
// @Description Принимает ID расшифровки и либо variant_id вместе с variant_task_id, либо legacy task_id из grading catalog. В variant-пути сервер проверяет владельца, берёт исторический снимок задания и роль main/basic. Assessment не сохраняет результат.
// @ID evaluateAnswer
// @Tags Грейдинг и фидбэк
// @Security accessCookie
// @Accept json
// @Produce json
// @Param request body EvaluateRequest true "Расшифровка и позиция сохранённого варианта"
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

	variantID := strings.TrimSpace(req.VariantID)
	variantTaskID := strings.TrimSpace(req.VariantTaskID)
	if variantID == "" || variantTaskID == "" {
		httpx.Error(r.Context(), w, fault.Validation("variant_id", "Укажите variant_id и variant_task_id."))
		return
	}

	result, err := h.service.EvaluateVariant(r.Context(), u.ID, req.TranscriptionID, variantID, variantTaskID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, EvaluateResponse{
		Score:            result.Score,
		MaxScore:         result.MaxScore,
		Verdict:          result.Verdict,
		CriterionResults: result.CriterionResults,
		Feedback:         result.Feedback,
	})
}

func optionalText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
