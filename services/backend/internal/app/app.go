// Package app is the composition root: it wires ports to adapters and exposes
// the HTTP router. Business policies belong to the feature application layers.
package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	llmgatewayv1 "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"

	authapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/infrastructure/argon2"
	authpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/infrastructure/postgres"
	authhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/auth/transport/http"
	graphapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/application"
	graphapi "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/infrastructure/courseapi"
	graphpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/infrastructure/postgres"
	graphhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/coursegraph/transport/http"
	geekapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	geekgraph "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/graph"
	geekllm "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/llm"
	geekpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/postgres"
	geekstorage "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/storage"
	geekhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/transport/http"
	materialsapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	materialsocr "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/ocr"
	materialspg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/postgres"
	materialstranslator "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/translator"
	materialshttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/transport/http"
	subjectapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/application"
	subjectpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/infrastructure/postgres"
	subjecthttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/transport/http"
	voiceapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/modelapi"
	voicepg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/postgres"
	voicehttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/transport/http"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/s3"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type App struct {
	cfg              Config
	pool             *pgxpool.Pool
	client           *http.Client
	now              func() time.Time
	hasher           authapp.PasswordHasher
	logger           *zap.Logger
	storage          *s3.Client
	materialsStorage materialsapp.S3Storage
	ocrWorker        *materialsapp.OCRWorker
	cleaningWorker   *materialsapp.CleaningWorker
	graphWorker      *graphapp.Worker
	// examiner is the LLM gateway over gRPC; the app owns its connection and
	// closes it in Close.
	examiner geekapp.Examiner
	gateway  io.Closer
	// speech voices Geek turns; one instance serves students and the warmer,
	// so concurrent misses of one question are synthesized once.
	speech *geekapp.SpeechCache
	// warmer voices prepared questions before students ask for them.
	warmer *geekapp.Warmer
}

// voiceName identifies the TTS settings in the keys of cached speech.
func (a *App) voiceName() string {
	return fmt.Sprintf("seed=%d cfg=%g steps=%d", a.cfg.TTSSeed, a.cfg.TTSCFGValue, a.cfg.TTSInferenceTimesteps)
}

func (a *App) models() *modelapi.Client {
	return modelapi.New(a.client, a.cfg.STTURL, a.cfg.TTSURL, a.cfg.VoiceTimeout, modelapi.TTSOptions{Seed: a.cfg.TTSSeed, CFGValue: a.cfg.TTSCFGValue, InferenceTimesteps: a.cfg.TTSInferenceTimesteps})
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
	a := &App{
		cfg:    cfg,
		pool:   pool,
		now:    time.Now,
		hasher: argon2.New(),
		logger: logger,
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	storage, err := s3.New(s3.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		PathStyle: cfg.S3PathStyle, CreateBucket: cfg.S3CreateBucket, Timeout: cfg.S3Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure S3 storage: %w", err)
	}
	a.storage = storage
	a.materialsStorage = &materialsStorageAdapter{storage: storage, bucket: cfg.S3Bucket}
	examiner, conn, err := geekllm.Dial(cfg.LLMGatewayAddress, cfg.LLMTimeout)
	if err != nil {
		return nil, err
	}
	a.examiner, a.gateway = examiner, conn

	materialRepository := materialspg.New(pool)
	materialsOCR := materialsocr.New(cfg.CourseGraphURL, cfg.CourseGraphTimeout)
	// OCR text is translated to Russian through the same LLM gateway as Geek.
	translator := materialstranslator.NewWithCompleter(materialstranslator.NewGateway(llmgatewayv1.NewGatewayClient(conn), cfg.LLMTimeout), 4)
	a.ocrWorker = materialsapp.NewOCRWorker(materialRepository, a.materialsStorage, materialsOCR, translator)
	a.cleaningWorker = materialsapp.NewCleaningWorker(materialRepository, a.materialsStorage, materialsOCR)
	a.speech = geekapp.NewSpeechCache(a.models(), geekstorage.NewAudio(storage, cfg.S3Bucket), a.voiceName())
	a.warmer = geekapp.NewWarmer(a.speech)
	a.graphWorker = graphapp.New(graphpg.New(pool), materialRepository, a.materialsStorage, graphapi.New(cfg.CourseGraphURL), a.warmer)

	return a, nil
}

// Close releases the app's connections once requests have finished.
func (a *App) Close() error {
	if a.gateway == nil {
		return nil
	}
	return a.gateway.Close()
}

// RunStorageSetup provisions the S3 bucket in the background, retrying until it
// succeeds or the caller cancels the context.
func (a *App) RunStorageSetup(ctx context.Context) error {
	backoff := time.Second
	for {
		ensureCtx, cancel := context.WithTimeout(ctx, a.cfg.S3Timeout)
		err := a.storage.EnsureBucket(ensureCtx)
		cancel()
		if err == nil {
			return nil
		}
		a.logger.Warn("S3 bucket is not ready", zap.Error(err), zap.Duration("retry_in", backoff))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, 30*time.Second)
	}
}

// RunBackground runs background workers and setup tasks.
func (a *App) RunBackground(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error { return a.RunStorageSetup(ctx) })
	group.Go(func() error { return a.ocrWorker.Run(ctx) })
	group.Go(func() error { return a.cleaningWorker.Run(ctx) })
	group.Go(func() error { return a.graphWorker.Run(ctx) })
	warmCtx := httpx.WithLogger(ctx, a.logger.With(zap.String("worker", "speech_warmer")))
	group.Go(func() error { return a.warmer.Run(warmCtx) })
	group.Go(func() error { a.warmFixture(warmCtx); return nil })
	return group.Wait()
}

// warmFixture queues the questions of the default graph: every course
// without its own graph is examined on it.
func (a *App) warmFixture(ctx context.Context) {
	if a.cfg.GeekGraphFixture == "" {
		return
	}
	g, ok, err := geekgraph.Fixture{Path: a.cfg.GeekGraphFixture}.Current(ctx, "")
	if err != nil || !ok {
		a.logger.Warn("graph fixture questions are not voiced ahead", zap.Error(err))
		return
	}
	questions := geekapp.MainQuestions(g)
	a.logger.Info("graph fixture questions queued for synthesis", zap.Int("queued", a.warmer.Prepare(questions)), zap.Int("questions", len(questions)))
}

type materialsStorageAdapter struct {
	storage *s3.Client
	bucket  string
}

func (m *materialsStorageAdapter) Put(ctx context.Context, key, contentType string, data []byte) error {
	return m.storage.PutObject(ctx, m.bucket, key, contentType, data)
}

func (m *materialsStorageAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	return m.storage.GetObject(ctx, m.bucket, key)
}

func (m *materialsStorageAdapter) Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	return m.storage.OpenObject(ctx, m.bucket, key)
}

func (m *materialsStorageAdapter) Delete(ctx context.Context, key string) error {
	return m.storage.DeleteObject(ctx, m.bucket, key)
}

func (a *App) Handler() http.Handler {
	auth := authapp.New(authpg.New(a.pool), a.hasher, a.now)
	authHandlers := authhttp.New(auth, a.now)
	models := a.models()
	voice := voiceapp.New(voicepg.New(a.pool), models, models, a.now)
	voiceHandlers := voicehttp.New(voice, a.cfg.MaxUploadBytes)
	subjectHandlers := subjecthttp.New(subjectapp.New(subjectpg.New(a.pool), security.IDGenerator{}, a.now, a.cfg.GeekGraphFixture != ""))

	materialsOCR := materialsocr.New(a.cfg.CourseGraphURL, a.cfg.CourseGraphTimeout)
	materialsService := materialsapp.New(materialspg.New(a.pool), a.materialsStorage, materialsOCR, a.now)
	// Course materials have their own per-file limit; BACKEND_MAX_UPLOAD_BYTES is the STT limit.
	materialsHandlers := materialshttp.NewHandler(materialsService, materialshttp.MaxUploadFileBytes)

	// The stored graph of stream A goes first once it exists; the fixture is the fallback.
	graphs := geekgraph.Chain{geekgraph.NewStored(a.pool), geekgraph.Fixture{Path: a.cfg.GeekGraphFixture}}
	geekService := geekapp.New(graphs, geekpg.New(a.pool), geekpg.NewTranscriptions(a.pool), a.examiner, a.speech,
		security.IDGenerator{}, a.now, geekapp.Config{MaxQuestions: a.cfg.GeekMaxQuestions})
	geekHandlers := geekhttp.New(geekService)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", authHandlers.Register)
	mux.HandleFunc("POST /auth/login", authHandlers.Login)
	mux.HandleFunc("POST /auth/teacher", authHandlers.LoginTeacher)
	mux.HandleFunc("POST /auth/refresh", authHandlers.Refresh)
	mux.HandleFunc("POST /auth/logout", authHandlers.Logout)
	mux.HandleFunc("GET /auth/me", authHandlers.Me)
	mux.Handle("POST /admin/subjects/{subject_id}/materials", protect(auth, http.HandlerFunc(materialsHandlers.UploadMaterials)))
	mux.Handle("GET /admin/subjects/{subject_id}/materials", protect(auth, http.HandlerFunc(materialsHandlers.ListMaterials)))
	graphHandlers := graphhttp.New(a.graphWorker)
	mux.Handle("GET /admin/subjects/{subject_id}/graph", protect(auth, http.HandlerFunc(graphHandlers.Read)))
	mux.Handle("POST /admin/subjects/{subject_id}/graph/build", protect(auth, http.HandlerFunc(graphHandlers.Build)))
	mux.Handle("DELETE /admin/subjects/{subject_id}/materials/{material_id}", protect(auth, http.HandlerFunc(materialsHandlers.DeleteMaterial)))
	mux.Handle("GET /admin/subjects/{subject_id}/materials/{material_id}/raw", protect(auth, http.HandlerFunc(materialsHandlers.DownloadRaw)))
	mux.Handle("POST /admin/subjects", protect(auth, http.HandlerFunc(subjectHandlers.Create)))
	mux.Handle("POST /voice/transcriptions", protect(auth, http.HandlerFunc(voiceHandlers.Transcribe)))
	mux.Handle("POST /voice/syntheses", protect(auth, http.HandlerFunc(voiceHandlers.Synthesize)))
	mux.Handle("GET /subjects", protect(auth, http.HandlerFunc(subjectHandlers.List)))
	mux.Handle("POST /diagnostic-sessions", protect(auth, http.HandlerFunc(geekHandlers.Start)))
	mux.Handle("GET /diagnostic-sessions", protect(auth, http.HandlerFunc(geekHandlers.History)))
	mux.Handle("GET /diagnostic-sessions/{id}", protect(auth, http.HandlerFunc(geekHandlers.Read)))
	mux.Handle("POST /diagnostic-sessions/{id}/answers", protect(auth, http.HandlerFunc(geekHandlers.Answer)))
	mux.Handle("GET /diagnostic-sessions/{id}/result", protect(auth, http.HandlerFunc(geekHandlers.Result)))
	mux.Handle("GET /diagnostic-sessions/{id}/turns/{turn_id}/audio", protect(auth, http.HandlerFunc(geekHandlers.TurnAudio)))
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(r.Context(), w, fault.New(fault.NotFound, "NOT_FOUND", "Ресурс не найден."))
	})
	return httpx.RequestLogger(a.logger, a.middleware(mux))
}

func protect(auth *authapp.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The admin panel sends its portal header and its own session cookie.
		access, _ := httpx.SessionCookieNames(r)
		principal, err := auth.Me(r.Context(), httpx.Cookie(r, access))
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
