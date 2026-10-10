package diagnostichttp

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/diagnostic"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type StartRequest struct {
	VariantID string `json:"variant_id" minLength:"1" maxLength:"128" binding:"required"`
}
type LearningStateResponse = diagnostic.LearningState

type Handler struct {
	service        *application.Service
	maxUploadBytes int64
}

func New(service *application.Service, maxUploadBytes int64) *Handler {
	return &Handler{service: service, maxUploadBytes: maxUploadBytes}
}

// Start creates a persistent diagnostic session for an owned variant.
// @Summary Начать диагностическую сессию
// @Description Сохраняет в PostgreSQL снимок порядка варианта. Повтор с тем же ключом возвращает ту же сессию.
// @Tags Диагностика
// @Security accessCookie
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param request body StartRequest true "Вариант для прохождения"
// @Success 201 {object} ProgressResponse
// @Success 200 {object} ProgressResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions [post]
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var request StartRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, reused, err := h.service.Start(r.Context(), principal.ID, request.VariantID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	status := http.StatusCreated
	if reused {
		status = http.StatusOK
	}
	httpx.JSON(w, status, progressResponse(result))
}

// LearningState returns the latest completed diagnostic for a subject.
// @Summary Состояние обучения по предмету
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param subject_id path string true "ID предмета"
// @Success 200 {object} LearningStateResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Router /subjects/{subject_id}/learning-state [get]
func (h *Handler) LearningState(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	state, err := h.service.LearningState(r.Context(), principal.ID, r.PathValue("subject_id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, state)
}

// Read returns status and the current public task.
// @Summary Прочитать состояние диагностики
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param id path string true "ID сессии" minlength(1) maxlength(128)
// @Success 200 {object} ProgressResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions/{id} [get]
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Read(r.Context(), principal.ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, progressResponse(result))
}

// CurrentAudio returns saved audio metadata for the current task without advancing the session.
// @Summary Прочитать состояние озвучки текущего задания
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param id path string true "ID сессии" minlength(1) maxlength(128)
// @Param variant_task_id query string true "ID текущего задания"
// @Success 200 {object} AudioMetadataResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/current/audio [get]
func (h *Handler) CurrentAudio(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	query := r.URL.Query()
	values, hasTaskID := query["variant_task_id"]
	if len(query) != 1 || !hasTaskID || len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		httpx.Error(r.Context(), w, fault.Validation("variant_task_id", "Укажите ID текущего задания."))
		return
	}
	metadata, err := h.service.CurrentAudio(r.Context(), principal.ID, r.PathValue("id"), values[0])
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, AudioMetadataResponse{VariantTaskID: metadata.VariantTaskID, Status: string(metadata.Status), AudioURL: metadata.AudioURL})
}

// RegenerateCurrentAudio synchronously repairs the saved audio for the current task.
// @Summary Восстановить озвучку текущего задания
// @Description Проверяет сохранённый объект и при необходимости синхронно восстанавливает только сохранённую инструкцию задания.
// @Tags Диагностика
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID сессии" minlength(1) maxlength(128)
// @Param request body AudioRegenerationRequest true "Текущее задание"
// @Success 200 {object} AudioMetadataResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/current/audio/regenerate [post]
func (h *Handler) RegenerateCurrentAudio(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var request AudioRegenerationRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	if strings.TrimSpace(request.VariantTaskID) == "" {
		httpx.Error(r.Context(), w, fault.Validation("variant_task_id", "Укажите ID текущего задания."))
		return
	}
	metadata, err := h.service.RegenerateCurrentAudio(r.Context(), principal.ID, r.PathValue("id"), request.VariantTaskID)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, AudioMetadataResponse{VariantTaskID: metadata.VariantTaskID, Status: string(metadata.Status), AudioURL: metadata.AudioURL})
}

// Answer transcribes and grades the audio for the current task.
// @Summary Отправить голосовой ответ
// @Description Выполняет STT, затем вызывает assessment application с variant_id, variant_task_id и transcription_id. Грейдер читает доверенный снимок и роль; ошибка оценки не меняет позицию.
// @Tags Диагностика
// @Security accessCookie
// @Accept mpfd
// @Produce json
// @Param id path string true "ID сессии" minlength(1) maxlength(128)
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param audio formData file true "WAV, Ogg/Opus или WebM/Opus, до 25 МиБ"
// @Param variant_task_id formData string true "ID текущей позиции варианта"
// @Success 200 {object} ProgressResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 415 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /diagnostic-sessions/{id}/answers [post]
func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	data, media, taskID, err := answerMultipart(w, r, h.maxUploadBytes)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Answer(r.Context(), principal.ID, r.PathValue("id"), taskID, key, data, media)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, progressResponse(result))
}

type SkipRequest struct {
	VariantTaskID string `json:"variant_task_id" minLength:"1" maxLength:"128" binding:"required"`
}

// Skip advances past the current task without speech recognition or grading.
// @Summary Пропустить вопрос диагностики
// @Tags Диагностика
// @Security accessCookie
// @Accept json
// @Produce json
// @Param id path string true "ID сессии"
// @Param Idempotency-Key header string true "Ключ повтора запроса"
// @Param request body SkipRequest true "Текущее задание"
// @Success 200 {object} ProgressResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/skip [post]
func (h *Handler) Skip(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var request SkipRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Skip(r.Context(), principal.ID, r.PathValue("id"), request.VariantTaskID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, progressResponse(result))
}

// Result returns the structured results of a completed session.
// @Summary Прочитать результат диагностики
// @Tags Диагностика
// @Security accessCookie
// @Produce json
// @Param id path string true "ID сессии" minlength(1) maxlength(128)
// @Success 200 {object} ResultResponse
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Router /diagnostic-sessions/{id}/result [get]
func (h *Handler) Result(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if err := noQuery(r); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	result, err := h.service.Result(r.Context(), principal.ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resultResponse(result))
}

func answerMultipart(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, string, string, error) {
	if maxBytes < 1 {
		return nil, "", "", fault.New(fault.Unavailable, "UPLOAD_LIMIT_UNAVAILABLE", "Лимит аудиозаписи не настроен.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+64*1024)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, "", "", httpx.BodyError(err)
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["audio"]) != 1 || len(r.MultipartForm.Value) != 1 || len(r.MultipartForm.Value["variant_task_id"]) != 1 {
		return nil, "", "", fault.Validation("multipart", "Передайте ровно поля audio и variant_task_id.")
	}
	header := r.MultipartForm.File["audio"][0]
	file, err := header.Open()
	if err != nil {
		return nil, "", "", httpx.BodyError(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, "", "", httpx.BodyError(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", "", fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Файл превышает 25 МиБ.")
	}
	media, params, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		media = ""
	}
	return data, mime.FormatMediaType(strings.ToLower(media), params), r.MultipartForm.Value["variant_task_id"][0], nil
}

func noQuery(r *http.Request) error {
	if r.URL.RawQuery != "" {
		return fault.Validation("query", "Параметры запроса не поддерживаются.")
	}
	return nil
}
