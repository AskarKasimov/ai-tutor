// Package app is the composition root: it wires ports to adapters and exposes
// the HTTP router. Business policies belong to the feature application layers.
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/infrastructure/argon2"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/infrastructure/hibp"
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
	auth := authapp.New(authpg.New(a.pool), a.hasher, hibp.New(a.client, a.cfg.PasswordCheckURL, a.cfg.PasswordCheckTimeout), a.now, authapp.Options{AuthRateLimit: a.cfg.AuthRateLimit, LoginEmailRateLimit: a.cfg.LoginEmailRateLimit})
	authHandlers := authhttp.New(auth, a.now)
	models := modelapi.New(a.client, a.cfg.STTURL, a.cfg.TTSURL, a.cfg.ProcessingTimeout)
	voice := voiceapp.New(voicepg.New(a.pool), models, models, a.now)
	voiceHandlers := voicehttp.New(voice, a.cfg.MaxUploadBytes)
	competency := competencyapp.New(competencypg.New(a.pool), &csvparser.Parser{}, a.now)
	competencyHandlers := competencyhttp.New(competency, a.cfg.MaxUploadBytes)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/auth/register", authHandlers.Register)
	mux.HandleFunc("POST /v1/auth/login", authHandlers.Login)
	mux.HandleFunc("POST /v1/auth/refresh", authHandlers.Refresh)
	mux.HandleFunc("POST /v1/auth/logout", authHandlers.Logout)
	mux.HandleFunc("GET /v1/auth/me", authHandlers.Me)
	mux.Handle("POST /v1/admin/competency-map/import", protect(auth, http.HandlerFunc(competencyHandlers.Import)))
	mux.Handle("POST /v1/voice/transcriptions", protect(auth, http.HandlerFunc(voiceHandlers.Transcribe)))
	mux.Handle("POST /v1/voice/syntheses", protect(auth, http.HandlerFunc(voiceHandlers.Synthesize)))
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

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.pool.Ping(ctx); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "ok"})
}
