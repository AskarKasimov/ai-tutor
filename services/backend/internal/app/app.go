// Package app is the composition root: it wires ports to adapters and exposes
// the HTTP router. Business policies belong to the feature application layers.
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	assessmentapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	assessmentmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/infrastructure/modelapi"
	assessmentpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/infrastructure/postgres"
	assessmenthttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/transport/http"
	authapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/infrastructure/argon2"
	authpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/infrastructure/postgres"
	authhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/transport/http"
	competencyapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/infrastructure/csvparser"
	competencypg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/infrastructure/postgres"
	competencyhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/competency/transport/http"
	voiceapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/modelapi"
	voicepg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/postgres"
	voicehttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/transport/http"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type App struct {
	cfg    Config
	pool   *pgxpool.Pool
	client *http.Client
	now    func() time.Time
	hasher *argon2.Hasher
}

func New(cfg Config, pool *pgxpool.Pool) (*App, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, fmt.Errorf("PostgreSQL pool is required")
	}
	return &App{cfg: cfg, pool: pool, now: time.Now, hasher: argon2.New(), client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (a *App) Handler() http.Handler {
	auth := authapp.New(authpg.New(a.pool), a.hasher, a.now)
	authHandlers := authhttp.New(auth, a.now)
	models := modelapi.New(a.client, a.cfg.STTURL, a.cfg.TTSURL, a.cfg.ProcessingTimeout)
	voice := voiceapp.New(voicepg.New(a.pool), models, models, a.now)
	voiceHandlers := voicehttp.New(voice, a.cfg.MaxUploadBytes)
	competency := competencyapp.New(competencypg.New(a.pool), &csvparser.Parser{}, a.now)
	competencyHandlers := competencyhttp.New(competency, a.cfg.MaxUploadBytes)
	assessment := assessmentapp.New(assessmentpg.New(a.pool), assessmentmodel.New(a.client, a.cfg.AssessmentBaseURL, a.cfg.AssessmentModel, a.cfg.AssessmentTimeout))
	assessmentHandlers := assessmenthttp.New(assessment)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", authHandlers.Register)
	mux.HandleFunc("POST /auth/login", authHandlers.Login)
	mux.HandleFunc("POST /auth/refresh", authHandlers.Refresh)
	mux.HandleFunc("POST /auth/logout", authHandlers.Logout)
	mux.HandleFunc("GET /auth/me", authHandlers.Me)
	mux.Handle("POST /admin/competency-map/import", protect(auth, http.HandlerFunc(competencyHandlers.Import)))
	mux.Handle("POST /voice/transcriptions", protect(auth, http.HandlerFunc(voiceHandlers.Transcribe)))
	mux.Handle("POST /voice/syntheses", protect(auth, http.HandlerFunc(voiceHandlers.Synthesize)))
	mux.Handle("POST /assessments/evaluate", protect(auth, http.HandlerFunc(assessmentHandlers.Evaluate)))
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, fault.New(fault.NotFound, "NOT_FOUND", "Ресурс не найден."))
	})
	return a.middleware(mux)
}

func protect(auth *authapp.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := auth.Me(r.Context(), httpx.Cookie(r, "access_token"))
		if err != nil {
			httpx.Error(w, err)
			return
		}
		next.ServeHTTP(w, httpx.WithPrincipal(r, principal))
	})
}

type HealthResponse struct {
	Status string `json:"status" enums:"ok"`
}

// health handles GET /health.
// @Summary Проверить API и подключение к PostgreSQL
// @ID health
// @Tags Сервис
// @Produce json
// @Success 200 {object} HealthResponse
// @Failure 503 {object} fault.Error
// @Router /health [get]
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.pool.Ping(ctx); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, 200, HealthResponse{Status: "ok"})
}
