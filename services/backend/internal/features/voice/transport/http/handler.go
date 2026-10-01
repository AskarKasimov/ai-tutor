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

type Handler struct {
	service        *application.Service
	maxUploadBytes int64
}

func New(service *application.Service, maxUploadBytes int64) *Handler {
	return &Handler{service: service, maxUploadBytes: maxUploadBytes}
}

func (h *Handler) Transcribe(w http.ResponseWriter, r *http.Request) {
	u, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	data, media, err := httpx.Upload(r, "audio", h.maxUploadBytes)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	tr, err := h.service.Transcribe(r.Context(), u.ID, data, media)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, Transcription{ID: tr.ID, Text: tr.Text, CreatedAt: tr.CreatedAt})
}

func (h *Handler) Synthesize(w http.ResponseWriter, r *http.Request) {
	if _, ok := httpx.Principal[user.User](r); !ok {
		httpx.Error(w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	data, err := h.service.Synthesize(r.Context(), req.Text)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
