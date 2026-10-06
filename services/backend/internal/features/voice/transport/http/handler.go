package voicehttp

import (
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type Transcription struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

type SynthesizeRequest struct {
	Text string `json:"text" minLength:"1" maxLength:"500" binding:"required"`
}

type Handler struct {
	service        *application.Service
	maxUploadBytes int64
}

func New(service *application.Service, maxUploadBytes int64) *Handler {
	return &Handler{service: service, maxUploadBytes: maxUploadBytes}
}

// Transcribe handles POST /voice/transcriptions.
// @Summary Распознать голосовую запись
// @ID transcribeVoice
// @Tags Голос
// @Security accessCookie
// @Accept mpfd
// @Produce json
// @Param audio formData file true "WAV, Ogg/Opus или WebM/Opus, до 25 МиБ"
// @Success 200 {object} Transcription
// @Failure 401 {object} fault.Error
// @Failure 413 {object} fault.Error
// @Failure 415 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /voice/transcriptions [post]
func (h *Handler) Transcribe(w http.ResponseWriter, r *http.Request) {
	u, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	data, media, err := httpx.Upload(r, "audio", h.maxUploadBytes)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	tr, err := h.service.Transcribe(r.Context(), u.ID, data, media)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, Transcription{ID: tr.ID, Text: tr.Text, CreatedAt: tr.CreatedAt})
}

// Synthesize handles POST /voice/syntheses.
// @Summary Озвучить текст
// @ID synthesizeQuestion
// @Tags Голос
// @Security accessCookie
// @Accept json
// @Produce audio/wav
// @Param request body SynthesizeRequest true "Текст для озвучивания"
// @Success 200 {file} file "Готовое WAV аудио"
// @Failure 401 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 502 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Failure 504 {object} fault.Error
// @Router /voice/syntheses [post]
func (h *Handler) Synthesize(w http.ResponseWriter, r *http.Request) {
	if _, ok := httpx.Principal[user.User](r); !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	var req SynthesizeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	data, err := h.service.Synthesize(r.Context(), req.Text)
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
