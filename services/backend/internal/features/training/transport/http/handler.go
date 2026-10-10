package traininghttp

import (
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"io"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	service  *application.Service
	maxBytes int64
}

func New(service *application.Service, maxBytes int64) *Handler { return &Handler{service, maxBytes} }

type StartRequest struct {
	PlanRevision *int64 `json:"plan_revision,omitempty" binding:"optional"`
}
type AudioResponse struct {
	ExerciseID string  `json:"exercise_id"`
	Status     string  `json:"status"`
	AudioURL   *string `json:"audio_url" extensions:"x-nullable"`
}
type HistoryResponse struct {
	Items      []training.Attempt    `json:"items"`
	Targets    []training.TargetView `json:"targets"`
	NextCursor string                `json:"next_cursor,omitempty" binding:"optional"`
}

func owner(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется авторизация."))
	}
	return p.ID, ok
}
func noQuery(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.RawQuery != "" {
		httpx.Error(r.Context(), w, fault.Validation("query", "Параметры запроса не поддерживаются."))
		return false
	}
	return true
}

// Preview returns training topics without creating a session.
// @Summary Темы предстоящей тренировки
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID диагностики"
// @Success 200 {object} training.Preview
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions/{id}/training/preview [get]
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	v, err := h.service.Preview(r.Context(), id, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, v)
}

// Start creates one persistent training per completed diagnostic.
// @Summary Начать тренировку
// @Tags Тренировка
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID диагностики"
// @Param Idempotency-Key header string true "Ключ запуска"
// @Param request body StartRequest true "Ревизия preview для свободной тренировки"
// @Success 201 {object} training.Progress
// @Success 200 {object} training.Progress
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions/{id}/training [post]
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var input StartRequest
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	v, reused, err := h.service.Start(r.Context(), id, r.PathValue("id"), r.Header.Get("Idempotency-Key"), input.PlanRevision)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	status := 201
	if reused {
		status = 200
	}
	httpx.JSON(w, status, v)
}

// ByDiagnostic retrieves a previously created training.
// @Summary Тренировка по диагностике
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID диагностики"
// @Success 200 {object} training.Progress
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /diagnostic-sessions/{id}/training [get]
func (h *Handler) ByDiagnostic(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	v, err := h.service.ByDiagnostic(r.Context(), id, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, v)
}

// Get reads progress and the saved current exercise.
// @Summary Текущее упражнение и прогресс
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID тренировки"
// @Success 200 {object} training.Progress
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /training-sessions/{id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	v, err := h.service.Get(r.Context(), id, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, v)
}

// Answer grades a saved exercise and advances the infinite cycle.
// @Summary Ответить на упражнение
// @Tags Тренировка
// @Security accessCookie
// @Accept mpfd
// @Produce json
// @Param id path string true "ID тренировки"
// @Param Idempotency-Key header string true "Ключ ответа"
// @Param exercise_id formData string true "Текущее упражнение"
// @Param audio formData file true "Голосовой ответ"
// @Success 200 {object} training.Progress
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 415 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /training-sessions/{id}/answers [post]
func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		httpx.Error(r.Context(), w, httpx.BodyError(err))
		return
	}
	defer r.MultipartForm.RemoveAll()
	form := r.MultipartForm
	if len(form.File) != 1 || len(form.File["audio"]) != 1 || len(form.Value) != 1 || len(form.Value["exercise_id"]) != 1 {
		httpx.Error(r.Context(), w, fault.Validation("multipart", "Передайте ровно audio и exercise_id."))
		return
	}
	header := form.File["audio"][0]
	file, err := header.Open()
	if err != nil {
		httpx.Error(r.Context(), w, httpx.BodyError(err))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, h.maxBytes+1))
	if err != nil {
		httpx.Error(r.Context(), w, httpx.BodyError(err))
		return
	}
	if int64(len(data)) > h.maxBytes {
		httpx.Error(r.Context(), w, fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Файл превышает допустимый размер."))
		return
	}
	media, params, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	v, err := h.service.Answer(r.Context(), id, r.PathValue("id"), form.Value["exercise_id"][0], r.Header.Get("Idempotency-Key"), data, mime.FormatMediaType(strings.ToLower(media), params))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, v)
}

type SkipRequest struct {
	ExerciseID string `json:"exercise_id" minLength:"1" maxLength:"128" binding:"required"`
}

// Skip advances to the next exercise without speech recognition or grading.
// @Summary Пропустить вопрос тренировки
// @Tags Тренировка
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID сессии"
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param request body SkipRequest true "Текущее упражнение"
// @Success 200 {object} training.Progress
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /training-sessions/{id}/skip [post]
func (h *Handler) Skip(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var request SkipRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	v, err := h.service.Skip(r.Context(), id, r.PathValue("id"), request.ExerciseID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, v)
}

// ResetAnswer clears a failed, non-active answer reservation so a new recording can be submitted.
// @Summary Сбросить неудачную отправку ответа
// @Tags Тренировка
// @Security accessCookie
// @Param id path string true "ID тренировки"
// @Success 204
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Router /training-sessions/{id}/answers/reset [post]
func (h *Handler) ResetAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok || !noQuery(w, r) {
		return
	}
	if err := h.service.ResetReservation(r.Context(), id, r.PathValue("id")); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// History reads a bounded page of accepted answers.
// @Summary История ответов тренировки
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID тренировки"
// @Param limit query int false "Размер страницы" default(20) minimum(1) maximum(100)
// @Param cursor query string false "Курсор"
// @Success 200 {object} HistoryResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /training-sessions/{id}/history [get]
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	q, err := httpx.ParseQuery(r, "limit", "cursor")
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	limit := 20
	if raw := q.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			httpx.Error(r.Context(), w, fault.Validation("limit", "Размер страницы от 1 до 100."))
			return
		}
	}
	before := int64(math.MaxInt64)
	if raw := q.Get("cursor"); raw != "" {
		before, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || before < 1 {
			httpx.Error(r.Context(), w, fault.Validation("cursor", "Некорректный курсор."))
			return
		}
	}
	items, err := h.service.History(r.Context(), id, r.PathValue("id"), before, limit+1)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	progress, err := h.service.Get(r.Context(), id, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	response := HistoryResponse{Items: items, Targets: progress.Targets}
	if len(items) > limit {
		response.Items = items[:limit]
		response.NextCursor = strconv.FormatInt(items[limit-1].Sequence, 10)
	}
	httpx.JSON(w, 200, response)
}

// Audio reads saved current exercise audio metadata.
// @Summary Озвучка текущего упражнения
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID тренировки"
// @Param exercise_id query string true "Текущее упражнение"
// @Success 200 {object} AudioResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /training-sessions/{id}/current/audio [get]
func (h *Handler) Audio(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	q, err := httpx.ParseQuery(r, "exercise_id")
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	exercise := q.Get("exercise_id")
	if exercise == "" {
		httpx.Error(r.Context(), w, fault.Validation("exercise_id", "Укажите текущее упражнение."))
		return
	}
	m, err := h.service.CurrentAudio(r.Context(), id, r.PathValue("id"), exercise)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, AudioResponse{ExerciseID: exercise, Status: string(m.Status), AudioURL: m.AudioURL})
}

// RegenerateAudio retries generation for the current exercise.
// @Summary Повторить озвучку текущего упражнения
// @Tags Тренировка
// @Security accessCookie
// @Produce json
// @Param id path string true "ID тренировки"
// @Param exercise_id query string true "Текущее упражнение"
// @Success 200 {object} AudioResponse
// @Router /training-sessions/{id}/current/audio/regenerate [post]
func (h *Handler) RegenerateAudio(w http.ResponseWriter, r *http.Request) {
	id, ok := owner(w, r)
	if !ok {
		return
	}
	q, err := httpx.ParseQuery(r, "exercise_id")
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	exercise := q.Get("exercise_id")
	if exercise == "" {
		httpx.Error(r.Context(), w, fault.Validation("exercise_id", "Укажите текущее упражнение."))
		return
	}
	m, err := h.service.RegenerateCurrentAudio(r.Context(), id, r.PathValue("id"), exercise)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, AudioResponse{ExerciseID: exercise, Status: string(m.Status), AudioURL: m.AudioURL})
}
