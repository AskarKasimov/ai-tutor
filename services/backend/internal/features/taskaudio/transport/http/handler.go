package taskaudiohttp

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type FileService interface {
	Open(context.Context, string, string) (io.ReadCloser, application.ObjectInfo, error)
}

type Handler struct{ service FileService }

func New(service FileService) *Handler { return &Handler{service: service} }

// File streams a saved WAV for an authenticated user with access to its task.
// @Summary Получить сохранённую озвучку задания
// @Tags Задания
// @Security accessCookie
// @Produce audio/wav
// @Param id path string true "ID аудиозаписи задания" minlength(1) maxlength(256)
// @Success 200 {file} file "Сохранённая WAV-озвучка"
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 409 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /task-audio/{id}/file [get]
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal[user.User](r)
	if !ok {
		httpx.Error(r.Context(), w, fault.New(fault.Unauthorized, "UNAUTHORIZED", "Требуется действующая сессия."))
		return
	}
	if len(r.URL.Query()) != 0 {
		httpx.Error(r.Context(), w, fault.Validation("query", "Параметры запроса не поддерживаются."))
		return
	}
	body, info, err := h.service.Open(r.Context(), principal.ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(r.Context(), w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}
