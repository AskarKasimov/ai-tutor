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
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/llm-gateway/internal/gateway"
	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/llmgateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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
	httpListener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		slog.Error("bind HTTP listener", "error", err)
		os.Exit(1)
	}
	grpcListener, err := net.Listen("tcp", cfg.GRPCListenAddress)
	if err != nil {
		_ = httpListener.Close()
		slog.Error("bind gRPC listener", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- service.Run(ctx) }()
	server := &http.Server{Addr: cfg.ListenAddress, Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: cfg.RequestDeadline + 15*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(cfg.GRPCMaxReceiveBytes), grpc.MaxSendMsgSize(cfg.GRPCMaxSendBytes), grpc.MaxConcurrentStreams(uint32(cfg.GRPCMaxConcurrentStreams)), grpc.MaxHeaderListSize(16<<10))
	pb.RegisterGatewayServer(grpcServer, gateway.NewGRPCServer(service))
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	serverErr := make(chan error, 2)
	go func() { serverErr <- server.Serve(httpListener) }()
	go func() { serverErr <- grpcServer.Serve(grpcListener) }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	done := false
	fatal := false
	runFinished := false
	for !done {
		select {
		case err = <-runErr:
			runFinished = true
			if ctx.Err() == nil {
				fatal = true
			}
			if err != nil {
				slog.Error("gateway runtime failed", "error", err)
			}
			done = true
		case err = <-serverErr:
			if ctx.Err() == nil {
				fatal = true
			}
			if !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, grpc.ErrServerStopped) {
				slog.Error("gateway listener failed", "error", err)
			}
			done = true
		case <-ticker.C:
			if service.Ready() {
				healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
			} else {
				healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
			}
		case <-ctx.Done():
			done = true
		}
	}
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), cfg.GracePeriod)
	defer stop()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = server.Shutdown(shutdownCtx) }()
	go func() {
		defer wg.Done()
		stopped := make(chan struct{})
		go func() { grpcServer.GracefulStop(); close(stopped) }()
		select {
		case <-stopped:
		case <-shutdownCtx.Done():
			grpcServer.Stop()
		}
	}()
	wg.Wait()
	if !runFinished {
		select {
		case runResult := <-runErr:
			if runResult != nil {
				fatal = true
				slog.Error("gateway runtime failed", "error", runResult)
			}
		case <-shutdownCtx.Done():
		}
	}
	service.Close()
	if fatal || err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, grpc.ErrServerStopped) {
		os.Exit(1)
	}
}
