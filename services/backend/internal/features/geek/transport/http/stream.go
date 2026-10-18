package geek

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type streamResponse struct {
	ctx     context.Context
	w       http.ResponseWriter
	opened  bool
	started time.Time
	first   bool
}

func newStreamResponse(ctx context.Context, w http.ResponseWriter) *streamResponse {
	return &streamResponse{ctx: ctx, w: w, started: time.Now()}
}
func (s *streamResponse) emit(e audio.StreamEvent) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	controller := http.NewResponseController(s.w)
	_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	if !s.opened {
		s.opened = true
		s.w.Header().Set("Content-Type", "application/x-ndjson")
		s.w.Header().Set("Cache-Control", "no-store")
		s.w.Header().Set("X-Accel-Buffering", "no")
		s.w.Header().Del("Content-Length")
		s.w.WriteHeader(http.StatusOK)
	}
	if err := json.NewEncoder(s.w).Encode(e); err != nil {
		return err
	}
	if err := controller.Flush(); err != nil {
		return err
	}
	if e.Type == "audio" && !s.first {
		s.first = true
		httpx.LoggerFromContext(s.ctx).Info("geek stream first audio", zap.Duration("ttfa", time.Since(s.started)))
	}
	return nil
}

// TurnAudioStream handles authenticated progressive PCM for a Geek turn.
// @Summary Поток голоса реплики Geek
// @Description NDJSON: start(version=1,sample_rate,channels=1,encoding=pcm_s16le), audio(seq с 0,data base64), end(chunks,samples) или error(code). Максимум 64 МиБ PCM и 256 КиБ на строку. Без part — вся реплика. Отключённый поток возвращает 406. Авторизация и проверка части выполняются до кеша/TTS. Ошибка после открытия — error и закрытие без end.
// @ID getDiagnosticTurnAudioStream
// @Tags Диагностика
// @Security accessCookie
// @Produce application/x-ndjson
// @Param id path string true "ID диагностики" minlength(1) maxlength(128)
// @Param turn_id path string true "ID реплики Geek" minlength(1) maxlength(128)
// @Param part query string false "Часть реплики" Enums(reply, question)
// @Success 200 {string} string "NDJSON PCM stream"
// @Failure 401 {object} fault.Error
// @Failure 404 {object} fault.Error
// @Failure 406 {object} fault.Error
// @Failure 422 {object} fault.Error
// @Failure 503 {object} fault.Error
// @Router /diagnostic-sessions/{id}/turns/{turn_id}/audio/stream [get]
func (h *Handler) TurnAudioStream(w http.ResponseWriter, r *http.Request) {
	u, ok := principal(w, r)
	if !ok {
		return
	}
	s := newStreamResponse(r.Context(), w)
	err := h.service.TurnAudioStream(r.Context(), u.ID, r.PathValue("id"), r.PathValue("turn_id"), application.AudioPart(r.URL.Query().Get("part")), s.emit)
	if err == nil {
		return
	}
	httpx.LoggerFromContext(r.Context()).Warn("geek stream failed", zap.Error(err), zap.Bool("opened", s.opened))
	if s.opened {
		_ = s.emit(audio.StreamEvent{Type: "error", Code: "SYNTHESIS_FAILED"})
		return
	}
	var f *fault.Error
	if !errors.As(err, &f) {
		err = errors.Join(fault.New(fault.Unavailable, "SYNTHESIS_FAILED", "Сервис синтеза недоступен."), err)
	}
	httpx.Error(r.Context(), w, err)
}
