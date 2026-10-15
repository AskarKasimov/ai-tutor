// Package app is the composition root: it wires ports to adapters and exposes
// the HTTP router. Business policies belong to the feature application layers.
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/training"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

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
	diagnosticapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/application"
	diagnosticpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/infrastructure/postgres"
	diagnostichttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/transport/http"
	diagnosticfeedbackapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/application"
	diagnosticfeedbackmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/infrastructure/modelapi"
	diagnosticfeedbackpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/infrastructure/postgres"
	diagnosticfeedbackhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnosticfeedback/transport/http"
	learningapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/application"
	learningmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/infrastructure/modelapi"
	learningpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/infrastructure/postgres"
	learninghttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/learning/transport/http"
	materialsapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
	materialsocr "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/ocr"
	materialspg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/postgres"
	materialss3 "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/s3"
	materialshttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/transport/http"
	subjectapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/application"
	subjectpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/infrastructure/postgres"
	subjecthttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/subject/transport/http"
	taskaudioapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/application"
	taskaudiopg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/infrastructure/postgres"
	taskaudios3 "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/infrastructure/s3"
	taskaudiohttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskaudio/transport/http"
	taskbankapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/application"
	taskbankpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/infrastructure/postgres"
	taskbankhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskbank/transport/http"
	taskgenapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/application"
	taskgenmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/infrastructure/modelapi"
	taskgenpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/infrastructure/postgres"
	taskgenhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/taskgen/transport/http"
	trainingapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/application"
	trainingmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/modelapi"
	trainingpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/postgres"
	trainingsnapshot "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/infrastructure/snapshot"
	traininghttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/training/transport/http"
	variantgenapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/application"
	variantgenpg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/infrastructure/postgres"
	variantgenrandom "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/infrastructure/random"
	variantgenhttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/variantgen/transport/http"
	voiceapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/application"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/modelapi"
	voicepg "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/infrastructure/postgres"
	voicehttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/voice/transport/http"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type App struct {
	cfg               Config
	pool              *pgxpool.Pool
	client            *http.Client
	now               func() time.Time
	hasher            authapp.PasswordHasher
	logger            *zap.Logger
	variantRepository variantgenapp.Repository
	diagnosticStore   diagnosticapp.Store
	audioWorker       interface {
		Run(context.Context) error
		ProcessOne(context.Context) (bool, error)
		Regenerate(context.Context, string) (audioasset.Asset, error)
		Prioritize(context.Context, string) error
	}
	audioStorage taskaudioapp.Storage
	materialsStorage materialsapp.S3Storage
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
		cfg:               cfg,
		pool:              pool,
		now:               time.Now,
		hasher:            argon2.New(),
		logger:            logger,
		variantRepository: variantgenpg.New(pool),
		diagnosticStore:   diagnosticpg.New(pool),
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	storage, err := taskaudios3.New(taskaudios3.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		PathStyle: cfg.S3PathStyle, CreateBucket: cfg.S3CreateBucket, Timeout: cfg.S3Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure task audio storage: %w", err)
	}
	materialsStorage, err := materialss3.New(materialss3.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		PathStyle: cfg.S3PathStyle, CreateBucket: cfg.S3CreateBucket, Timeout: cfg.S3Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure materials S3 storage: %w", err)
	}
	synth := modelapi.New(a.client, cfg.STTURL, cfg.TTSURL, cfg.VoiceTimeout, modelapi.TTSOptions{Seed: cfg.TTSSeed, CFGValue: cfg.TTSCFGValue, InferenceTimesteps: cfg.TTSInferenceTimesteps})
	a.audioWorker = taskaudioapp.NewWorker(taskaudiopg.New(pool), storage, synth, taskaudioapp.WorkerConfig{
		Concurrency: cfg.AudioWorkers, PollInterval: cfg.AudioPollInterval, VoiceTimeout: cfg.VoiceTimeout,
		S3Timeout: cfg.S3Timeout, Bucket: cfg.S3Bucket,
	})
	a.audioStorage = storage
	a.materialsStorage = materialsStorage
	return a, nil
}

func (a *App) Handler() http.Handler {
	auth := authapp.New(authpg.New(a.pool), a.hasher, a.now)
	authHandlers := authhttp.New(auth, a.now)
	models := modelapi.New(a.client, a.cfg.STTURL, a.cfg.TTSURL, a.cfg.VoiceTimeout, modelapi.TTSOptions{Seed: a.cfg.TTSSeed, CFGValue: a.cfg.TTSCFGValue, InferenceTimesteps: a.cfg.TTSInferenceTimesteps})
	grader := assessmentmodel.New(a.client, a.cfg.AssessmentBaseURL, a.cfg.AssessmentModel, a.cfg.AssessmentTimeout)
	feedbackSynthesizer := diagnosticfeedbackmodel.New(a.client, a.cfg.AssessmentBaseURL, a.cfg.AssessmentModel, a.cfg.AssessmentTimeout)
	taskGenerator := taskgenmodel.New(a.client, a.cfg.TaskgenBaseURL, a.cfg.TaskgenModel, a.cfg.TaskgenTimeout)
	voice := voiceapp.New(voicepg.New(a.pool), models, models, a.now)
	voiceHandlers := voicehttp.New(voice, a.cfg.MaxUploadBytes)
	competency := competencyapp.New(competencypg.New(a.pool), &csvparser.Parser{}, a.now)
	competencyHandlers := competencyhttp.New(competency, a.cfg.MaxUploadBytes)
	subjectHandlers := subjecthttp.New(subjectapp.New(subjectpg.New(a.pool), security.IDGenerator{}, a.now))
	assessmentService := assessmentapp.New(assessmentpg.New(a.pool), a.variantRepository, grader)
	assessmentHandlers := assessmenthttp.New(assessmentService)
	taskbankHandlers := taskbankhttp.New(taskbankapp.New(taskbankpg.New(a.pool)))
	taskgenRepository := taskgenpg.New(a.pool)
	taskgenHandlers := taskgenhttp.New(
		taskgenapp.New(taskgenRepository, taskGenerator, a.cfg.TaskgenModel, func() int64 { return a.now().Unix() }),
		taskgenapp.NewMaterialService(taskgenRepository, func() int64 { return a.now().Unix() }),
	)
	variantgenHandlers := variantgenhttp.New(variantgenapp.New(a.variantRepository, variantgenrandom.Chooser{}, security.IDGenerator{}, a.now))
	taskAudioService := taskaudioapp.NewService(taskaudiopg.New(a.pool), a.audioStorage).WithPrioritizer(a.audioWorker)
	taskAudioHandlers := taskaudiohttp.New(taskAudioService)
	diagnosticService := diagnosticapp.New(a.diagnosticStore, a.variantRepository, voice, assessmentService, security.IDGenerator{}, a.now).WithAudioReader(taskAudioService).WithAudioRegenerator(a.audioWorker).WithTranscriptions(assessmentpg.New(a.pool))
	diagnosticHandlers := diagnostichttp.New(diagnosticService, a.cfg.MaxUploadBytes)
	trainingService := trainingapp.New(trainingpg.New(a.pool), a.diagnosticStore, voice, trainingGrader{assessmentService}, trainingsnapshot.Provider{}, security.IDGenerator{}, a.now).WithAudio(taskAudioService).WithAudioRegenerator(a.audioWorker)
	trainingHandlers := traininghttp.New(trainingService, a.cfg.MaxUploadBytes)
	chatRepository := trainingpg.NewChatRepository(a.pool).WithLease(a.cfg.VoiceTimeout + a.cfg.AssessmentTimeout + a.cfg.TutorTimeout + 30*time.Second)
	trainingService = trainingService.WithCheckedNext(chatRepository)
	var tutor trainingapp.Tutor
	if a.cfg.TutorEnabled {
		tutor = trainingmodel.NewTutor(a.client, a.cfg.TutorBaseURL, a.cfg.TutorModel, a.cfg.TutorTimeout)
	}
	chatService := trainingapp.NewChatService(chatRepository, voice, tutor, trainingFinalGrader{assessmentService}, security.IDGenerator{}, a.now).
		WithContextLimits(training.ContextLimits{TaskMaxChars: a.cfg.TutorTaskMaxChars, HistoryMaxChars: a.cfg.TutorHistoryMaxChars, SummaryMaxChars: a.cfg.TutorSummaryMaxChars})
	chatHandlers := traininghttp.NewChatHandler(chatService, a.cfg.MaxUploadBytes)
	var teacher learningapp.Teacher
	if a.cfg.TutorEnabled {
		teacher = learningmodel.New(a.client, a.cfg.TutorBaseURL, a.cfg.TutorModel, a.cfg.TutorTimeout)
	}
	learningHandlers := learninghttp.New(learningapp.New(learningpg.New(a.pool), a.diagnosticStore, voice, teacher, security.IDGenerator{}, a.now).WithTranscriptions(assessmentpg.New(a.pool)), a.cfg.MaxUploadBytes)
	feedbackService := diagnosticfeedbackapp.New(diagnosticService, feedbackSynthesizer, diagnosticfeedbackpg.NewStore(a.pool), a.now).
		WithTaskFinder(diagnosticfeedbackpg.NewTaskFinder(a.pool))
	feedbackHandlers := diagnosticfeedbackhttp.New(feedbackService)

	materialsOCR := materialsocr.New(a.cfg.OCRURL, a.cfg.OCRTimeout)
	materialsService := materialsapp.New(materialspg.New(a.pool), a.materialsStorage, materialsOCR, a.now)
	materialsHandlers := materialshttp.NewHandler(materialsService, a.cfg.MaxUploadBytes)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", authHandlers.Register)
	mux.HandleFunc("POST /auth/login", authHandlers.Login)
	mux.HandleFunc("POST /auth/teacher", authHandlers.LoginTeacher)
	mux.HandleFunc("POST /auth/refresh", authHandlers.Refresh)
	mux.HandleFunc("POST /auth/logout", authHandlers.Logout)
	mux.HandleFunc("GET /auth/me", authHandlers.Me)
	mux.Handle("POST /admin/subjects/{subject_id}/competency-map/import", protect(auth, http.HandlerFunc(competencyHandlers.ImportSubject)))
	mux.Handle("POST /admin/subjects/{subject_id}/materials", protect(auth, http.HandlerFunc(materialsHandlers.UploadMaterials)))
	mux.Handle("GET /admin/subjects/{subject_id}/materials", protect(auth, http.HandlerFunc(materialsHandlers.ListMaterials)))
	mux.Handle("DELETE /admin/subjects/{subject_id}/materials/{material_id}", protect(auth, http.HandlerFunc(materialsHandlers.DeleteMaterial)))
	mux.Handle("GET /admin/subjects/{subject_id}/materials/{material_id}/raw", protect(auth, http.HandlerFunc(materialsHandlers.DownloadRaw)))
	mux.Handle("POST /admin/subjects", protect(auth, http.HandlerFunc(subjectHandlers.Create)))
	mux.Handle("POST /voice/transcriptions", protect(auth, http.HandlerFunc(voiceHandlers.Transcribe)))
	mux.Handle("POST /voice/syntheses", protect(auth, http.HandlerFunc(voiceHandlers.Synthesize)))
	mux.Handle("POST /assessments/evaluate", protect(auth, http.HandlerFunc(assessmentHandlers.Evaluate)))
	mux.Handle("POST /assessments/overall-feedback", protect(auth, http.HandlerFunc(feedbackHandlers.SynthesizeAnswersFeedback)))
	mux.Handle("POST /diagnostic-sessions", protect(auth, http.HandlerFunc(diagnosticHandlers.Start)))
	mux.Handle("GET /diagnostic-sessions/{id}", protect(auth, http.HandlerFunc(diagnosticHandlers.Read)))
	mux.Handle("GET /subjects/{subject_id}/learning-state", protect(auth, http.HandlerFunc(diagnosticHandlers.LearningState)))
	mux.Handle("GET /diagnostic-sessions/{id}/current/audio", protect(auth, http.HandlerFunc(diagnosticHandlers.CurrentAudio)))
	mux.Handle("POST /diagnostic-sessions/{id}/current/audio/regenerate", protect(auth, http.HandlerFunc(diagnosticHandlers.RegenerateCurrentAudio)))
	mux.Handle("GET /task-audio/{id}/file", protect(auth, http.HandlerFunc(taskAudioHandlers.File)))
	mux.Handle("POST /diagnostic-sessions/{id}/answers/transcription", protect(auth, http.HandlerFunc(diagnosticHandlers.AnswerTranscription)))
	mux.Handle("POST /diagnostic-sessions/{id}/answers", protect(auth, http.HandlerFunc(diagnosticHandlers.Answer)))
	mux.Handle("POST /diagnostic-sessions/{id}/skip", protect(auth, http.HandlerFunc(diagnosticHandlers.Skip)))
	mux.Handle("GET /diagnostic-sessions/{id}/result", protect(auth, http.HandlerFunc(diagnosticHandlers.Result)))
	mux.Handle("GET /diagnostic-sessions/{id}/feedback", protect(auth, http.HandlerFunc(feedbackHandlers.GetFeedback)))
	mux.Handle("GET /diagnostic-sessions/{id}/training/preview", protect(auth, http.HandlerFunc(trainingHandlers.Preview)))
	mux.Handle("POST /diagnostic-sessions/{id}/training", protect(auth, http.HandlerFunc(trainingHandlers.Start)))
	mux.Handle("GET /diagnostic-sessions/{id}/training", protect(auth, http.HandlerFunc(trainingHandlers.ByDiagnostic)))
	mux.Handle("GET /training-sessions/{id}", protect(auth, http.HandlerFunc(trainingHandlers.Get)))
	mux.Handle("POST /training-sessions/{id}/answers", protect(auth, http.HandlerFunc(trainingHandlers.Answer)))
	mux.Handle("POST /training-sessions/{id}/skip", protect(auth, http.HandlerFunc(trainingHandlers.Skip)))
	mux.Handle("POST /training-sessions/{id}/answers/reset", protect(auth, http.HandlerFunc(trainingHandlers.ResetAnswer)))
	mux.Handle("POST /training-sessions/{id}/exercises/{exercise_id}/next", protect(auth, http.HandlerFunc(trainingHandlers.NextChecked)))
	mux.Handle("GET /training-sessions/{id}/history", protect(auth, http.HandlerFunc(trainingHandlers.History)))
	mux.Handle("GET /training-sessions/{id}/current/audio", protect(auth, http.HandlerFunc(trainingHandlers.Audio)))
	mux.Handle("POST /training-sessions/{id}/current/audio/regenerate", protect(auth, http.HandlerFunc(trainingHandlers.RegenerateAudio)))
	mux.Handle("GET /training-sessions/{id}/exercises/{exercise_id}/chat", protect(auth, http.HandlerFunc(chatHandlers.Read)))
	mux.Handle("POST /training-sessions/{id}/exercises/{exercise_id}/chat/turns", protect(auth, http.HandlerFunc(chatHandlers.Send)))
	mux.Handle("POST /training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/retry", protect(auth, http.HandlerFunc(chatHandlers.Retry)))
	mux.Handle("POST /training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/cancel", protect(auth, http.HandlerFunc(chatHandlers.Cancel)))
	mux.Handle("GET /tasks", protect(auth, http.HandlerFunc(taskbankHandlers.Search)))
	mux.Handle("GET /subjects/{subject_id}/competency-map", protect(auth, http.HandlerFunc(competencyHandlers.ReadSubject)))
	mux.Handle("GET /subjects", protect(auth, http.HandlerFunc(subjectHandlers.List)))
	mux.Handle("GET /tasks/{id}", protect(auth, http.HandlerFunc(taskbankHandlers.Profile)))
	mux.Handle("POST /tasks/generate", protect(auth, http.HandlerFunc(taskgenHandlers.Generate)))
	mux.Handle("POST /admin/materials", protect(auth, http.HandlerFunc(taskgenHandlers.ImportMaterial)))
	mux.Handle("POST /variants", protect(auth, http.HandlerFunc(variantgenHandlers.Create)))
	mux.Handle("GET /variants", protect(auth, http.HandlerFunc(variantgenHandlers.List)))
	mux.Handle("GET /variants/{id}", protect(auth, http.HandlerFunc(variantgenHandlers.Read)))
	mux.Handle("GET /variants/{id}/tasks/{task_id}", protect(auth, http.HandlerFunc(variantgenHandlers.ReadTask)))
	mux.Handle("POST /diagnostic-sessions/{id}/learning-plan", protect(auth, http.HandlerFunc(learningHandlers.Create)))
	mux.Handle("GET /diagnostic-sessions/{id}/learning-plan", protect(auth, http.HandlerFunc(learningHandlers.ByDiagnostic)))
	mux.Handle("GET /learning-plans/{id}", protect(auth, http.HandlerFunc(learningHandlers.Get)))
	mux.Handle("GET /learning-plans/{id}/items/{index}/messages", protect(auth, http.HandlerFunc(learningHandlers.Messages)))
	mux.Handle("POST /learning-plans/{id}/open", protect(auth, http.HandlerFunc(learningHandlers.Open)))
	mux.Handle("POST /learning-plans/{id}/answers/transcription", protect(auth, http.HandlerFunc(learningHandlers.AnswerTranscription)))
	mux.Handle("POST /learning-plans/{id}/answers", protect(auth, http.HandlerFunc(learningHandlers.Answer)))
	mux.Handle("POST /learning-plans/{id}/answers/retry", protect(auth, http.HandlerFunc(learningHandlers.Retry)))
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
