package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/app"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
	"go.uber.org/zap"
)

// @title AI Tutor Backend API
// @version 0.1.0
// @description Реализованный HTTP API backend. Даты — Unix timestamp в секундах. Авторизация — HttpOnly Secure cookies.
// @servers.url https://localhost:8443/api/v1
// @servers.description Локальный API через Caddy (Docker Compose)
// @servers.url /api/v1
// @servers.description HTTPS reverse proxy
// @securityDefinitions.apikey accessCookie
// @in cookie
// @name access_token

// @securityDefinitions.apikey refreshCookie
// @in cookie
// @name refresh_token
func main() {
	logger, err := loggerConfig().Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "initialize logger:", err)
		os.Exit(1)
	}
	if err := run(logger); err != nil {
		logger.Error("API stopped", zap.Error(err))
		_ = logger.Sync()
		os.Exit(1)
	}
	_ = logger.Sync()
}
func loggerConfig() zap.Config {
	cfg := zap.NewProductionConfig()
	// Access logs must retain one entry per request, including bursts of errors.
	cfg.Sampling = nil
	return cfg
}

func run(logger *zap.Logger) error {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := &http.Client{Timeout: 3 * time.Second}
		resp, err := c.Get("http://127.0.0.1:8002/health")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("health: %d", resp.StatusCode)
		}
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] != "migrate" {
		return fmt.Errorf("usage: api [migrate|healthcheck]")
	}
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}
	defer pool.Close()
	migrationCtx, migrationCancel := context.WithTimeout(ctx, 30*time.Second)
	err = postgres.Migrate(migrationCtx, pool)
	migrationCancel()
	if err != nil {
		return err
	}
	if len(os.Args) > 1 {
		logger.Info("database migrations applied")
		return nil
	}
	a, err := app.New(cfg, pool, logger)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.ListenAddress, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: cfg.HTTPWriteTimeout(), IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 * 1024}
	stopped := make(chan error, 1)
	go func() {
		logger.Info("API listening", zap.String("address", cfg.ListenAddress))
		stopped <- server.ListenAndServe()
	}()
	select {
	case err = <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	}
}
