// Package app is the composition root: it wires ports to adapters and exposes
// the HTTP router. Business policies belong to the feature application layers.
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	assessmentapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/application"
	assessmentmock "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/assessment/infrastructure/mock"
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
	taskbankapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/application"
	taskbankpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/infrastructure/postgres"
	taskbankhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/transport/http"
	taskgenapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	taskgenmock "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/infrastructure/mock"
	taskgenmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/infrastructure/modelapi"
	taskgenpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/infrastructure/postgres"
	taskgenhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/transport/http"
	variantgenapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/application"
	variantgenpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/infrastructure/postgres"
	variantgenhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/transport/http"
	voiceapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/application"
	voicemock "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/mock"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/modelapi"
	voicepg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/postgres"
	voicehttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/transport/http"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

type App struct {
	cfg               Config
	pool              *pgxpool.Pool
	client            *http.Client
	now               func() time.Time
	hasher            *argon2.Hasher
	logger            *zap.Logger
	variantRepository variantgenapp.Repository
}

func New(cfg Config, pool *pgxpool.Pool, logger *zap.Logger) (*App, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, fmt.Errorf("PostgreSQL pool is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &App{cfg: cfg, pool: pool, now: time.Now, hasher: argon2.New(), logger: logger, variantRepository: variantgenpg.New(pool), client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (a *App) Handler() http.Handler {
	auth := authapp.New(authpg.New(a.pool), a.hasher, a.now)
	authHandlers := authhttp.New(auth, a.now)
	var recognizer voiceapp.Recognizer
	var synthesizer voiceapp.Synthesizer
	var grader assessmentapp.Grader
	var taskGenerator taskgenapp.Generator
	modelName := a.cfg.TaskgenModel
	if a.cfg.APIMode == "mock" {
		recognizer, synthesizer = voicemock.Client{}, voicemock.Client{}
		grader = assessmentmock.Grader{}
		taskGenerator, modelName = taskgenmock.Generator{}, "mock"
	} else {
		models := modelapi.New(a.client, a.cfg.STTURL, a.cfg.TTSURL, a.cfg.ProcessingTimeout)
		recognizer, synthesizer = models, models
		grader = assessmentmodel.New(a.client, a.cfg.AssessmentBaseURL, a.cfg.AssessmentModel, a.cfg.AssessmentTimeout)
		taskGenerator = taskgenmodel.New(a.client, a.cfg.TaskgenBaseURL, a.cfg.TaskgenModel, a.cfg.TaskgenTimeout)
	}
	voice := voiceapp.New(voicepg.New(a.pool), recognizer, synthesizer, a.now)
	voiceHandlers := voicehttp.New(voice, a.cfg.MaxUploadBytes)
	competency := competencyapp.New(competencypg.New(a.pool), &csvparser.Parser{}, a.now)
	competencyHandlers := competencyhttp.New(competency, a.cfg.MaxUploadBytes)
	assessment := assessmentapp.New(assessmentpg.New(a.pool), grader)
	assessmentHandlers := assessmenthttp.New(assessment)
	taskbankHandlers := taskbankhttp.New(taskbankapp.New(taskbankpg.New(a.pool)))
	taskgenRepository := taskgenpg.New(a.pool)
	taskgenHandlers := taskgenhttp.New(
		taskgenapp.New(taskgenRepository, taskGenerator, modelName, func() int64 { return a.now().Unix() }),
		taskgenapp.NewMaterialService(taskgenRepository, func() int64 { return a.now().Unix() }),
	)
	variantgenHandlers := variantgenhttp.New(variantgenapp.New(a.variantRepository, variantgenpg.Chooser{}, variantgenpg.IDs{}, a.now))

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
	mux.Handle("GET /tasks", protect(auth, http.HandlerFunc(taskbankHandlers.Search)))
	mux.Handle("GET /competency-map", protect(auth, http.HandlerFunc(competencyHandlers.Read)))
	mux.Handle("GET /tasks/{id}", protect(auth, http.HandlerFunc(taskbankHandlers.Profile)))
	mux.Handle("POST /tasks/generate", protect(auth, http.HandlerFunc(taskgenHandlers.Generate)))
	mux.Handle("POST /admin/materials", protect(auth, http.HandlerFunc(taskgenHandlers.ImportMaterial)))
	mux.Handle("POST /variants", protect(auth, http.HandlerFunc(variantgenHandlers.Create)))
	mux.Handle("GET /variants", protect(auth, http.HandlerFunc(variantgenHandlers.List)))
	mux.Handle("GET /variants/{id}", protect(auth, http.HandlerFunc(variantgenHandlers.Read)))
	mux.Handle("GET /variants/{id}/tasks/{task_id}", protect(auth, http.HandlerFunc(variantgenHandlers.ReadTask)))
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(r.Context(), w, fault.New(fault.NotFound, "NOT_FOUND", "Ресурс не найден."))
	})
	return httpx.RequestLogger(a.logger, a.middleware(mux))
}

// VariantTaskReader exposes owner-scoped historical task snapshots to grading services.
func (a *App) VariantTaskReader() variant.TaskReader { return a.variantRepository }

func protect(auth *authapp.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := auth.Me(r.Context(), httpx.Cookie(r, "access_token"))
		if err != nil {
			httpx.Error(r.Context(), w, err)
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
		httpx.Error(r.Context(), w, err)
		return
	}
	httpx.JSON(w, 200, HealthResponse{Status: "ok"})
}
