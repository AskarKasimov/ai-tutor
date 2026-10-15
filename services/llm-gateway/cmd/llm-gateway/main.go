// Package main runs the internal LLM Gateway.
//
// @title LLM Gateway API
// @version 1.0.0
// @description Internal server-to-server OpenAI-compatible completion API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/llm-gateway/internal/gateway"
)

func main() {
	configPath := os.Getenv("GATEWAY_CONFIG")
	if configPath == "" {
		configPath = "/etc/llm-gateway/config.json"
	}
	cfg, err := gateway.LoadConfig(configPath)
	if err != nil {
		slog.Error("invalid gateway configuration", "error", err)
		os.Exit(1)
	}
	service, err := gateway.NewService(cfg)
	if err != nil {
		slog.Error("initialize gateway", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- service.Run(ctx) }()
	server := &http.Server{Addr: cfg.ListenAddress, Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: cfg.RequestDeadline + 15*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	select {
	case err = <-runErr:
		if err != nil {
			slog.Error("gateway runtime failed", "error", err)
			os.Exit(1)
		}
	case err = <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("gateway HTTP server failed", "error", err)
			cancel()
			os.Exit(1)
		}
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.GracePeriod)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	service.Close()
}
