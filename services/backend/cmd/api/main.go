package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/app"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres"
)

// @title AI Tutor Backend API
// @version 0.1.0
// @description Реализованный HTTP API backend. Даты — Unix timestamp в секундах. Авторизация — HttpOnly Secure cookies.
// @servers.url /api/v1
// @servers.description HTTPS reverse proxy
// @securityDefinitions.apikey accessCookie
// @in cookie
// @name access_token

// @securityDefinitions.apikey refreshCookie
// @in cookie
// @name refresh_token
func main() {
	if err := run(); err != nil {
		slog.Error("API stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
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
		slog.Info("database migrations applied")
		return nil
	}
	a, err := app.New(cfg, pool)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.ListenAddress, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: cfg.ProcessingTimeout + 30*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 * 1024}
	stopped := make(chan error, 1)
	go func() { slog.Info("API listening", "address", cfg.ListenAddress); stopped <- server.ListenAndServe() }()
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
