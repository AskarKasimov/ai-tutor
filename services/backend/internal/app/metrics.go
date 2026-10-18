package app

import (
	"net/http"
)

// metrics exposes bounded operational series inside the backend network.
// @Summary Внутренние метрики голоса
// @Description Prometheus text. Доступен внутри backend-сети; Caddy не публикует эту операцию. Метки только из фиксированных перечислений, без текста, seed, session ID и хешей.
// @ID getVoiceMetrics
// @Tags Система
// @Produce text/plain
// @Success 200 {string} string "Prometheus metrics"
// @Router /metrics [get]
func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = a.voiceMetrics.WritePrometheus(w)
}
