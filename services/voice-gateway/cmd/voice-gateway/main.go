package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	pb "github.com/AskarKasimov/ai-tutor/services/shared/gen/go/voicegateway/v1"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/config"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/execution"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/gateway"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "voice-gateway"))
	if err := run(); err != nil {
		slog.Error("voice gateway stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
func run() error {
	path := os.Getenv("VOICE_GATEWAY_CONFIG")
	if path == "" {
		path = "config/dev.yaml"
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	var level slog.Level
	if err = level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Key = "ts"
		}
		return a
	}})).With("service", "voice-gateway")
	slog.SetDefault(logger)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		_, port, _ := net.SplitHostPort(cfg.HTTPAddress)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+port+"/health/ready", nil)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return errors.New("voice gateway is not ready")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return errors.New("voice gateway is not ready")
		}
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] == "release-held" {
		if len(os.Args) != 4 || os.Args[3] != "--upstream-stopped" {
			return errors.New("usage: voice-gateway release-held INSTANCE --upstream-stopped")
		}
		_, port, _ := net.SplitHostPort(cfg.HTTPAddress)
		payload, _ := json.Marshal(map[string]any{"instance": os.Args[2], "upstream_stopped": true})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:"+port+"/admin/release-held", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return errors.New("voice gateway unavailable")
		}
		defer response.Body.Close()
		if response.StatusCode != 204 {
			return errors.New("held slots could not be released")
		}
		return nil
	}
	if len(os.Args) > 1 {
		return errors.New("unknown voice gateway command")
	}
	blobs, err := journal.OpenBlobs(cfg.DataDir, cfg.MaxPayloadBytes)
	if err != nil {
		return err
	}
	defer blobs.Close()
	store, err := journal.New(cfg.Brokers, cfg.STTTopic, cfg.TTSTopic)
	if err != nil {
		return err
	}
	defer store.Close()
	queues, err := cfg.Queues()
	if err != nil {
		return err
	}
	engine := execution.New(store, blobs, queues, cfg.Retention.Duration, cfg.AbortOnDisconnect, logger)
	restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = engine.Restore(restoreCtx)
	restoreCancel()
	if err != nil {
		return err
	}
	provider := cfg.Provider()
	provider.Client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	voices := map[string]string{}
	for id, v := range cfg.Voices {
		voices[id] = v.Revision()
	}
	tokens := make(chan struct{}, cfg.MaxConnections)
	rpc := grpc.NewServer(grpc.MaxRecvMsgSize(26<<20), grpc.MaxSendMsgSize(65<<20), grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		select {
		case tokens <- struct{}{}:
			defer func() { <-tokens }()
		default:
			return nil, status.Error(codes.ResourceExhausted, "voice connection limit")
		}
		started := time.Now()
		response, err := handler(ctx, req)
		op := strings.TrimPrefix(info.FullMethod, "/voicegateway.v1.VoiceGateway/")
		logger.Log(ctx, requestLevel(err), "voice gateway request", slog.String("operation", op), slog.String("trace_id", traceID(ctx)), slog.String("code", status.Code(err).String()), slog.Duration("duration", time.Since(started)))
		return response, err
	}))
	pb.RegisterVoiceGatewayServer(rpc, gateway.New(engine, provider, voices, cfg.RequestTimeout.Duration))
	listener, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", engine.Metrics)
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if !engine.Ready() || store.Ping(ctx) != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("POST /admin/release-held", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Instance string `json:"instance"`
			Stopped  bool   `json:"upstream_stopped"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.Instance == "" || !input.Stopped {
			w.WriteHeader(400)
			return
		}
		if err := engine.ReleaseHeld(input.Instance); err != nil {
			w.WriteHeader(409)
			return
		}
		logger.Warn("voice held slots released by operator", slog.String("instance", input.Instance))
		w.WriteHeader(204)
	})
	httpServer := &http.Server{Addr: cfg.HTTPAddress, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	go engine.Maintain(ctx)
	failures := make(chan error, 2)
	go func() { failures <- rpc.Serve(listener) }()
	go func() { failures <- httpServer.ListenAndServe() }()
	logger.Info("voice gateway started", slog.String("grpc_address", cfg.GRPCAddress), slog.String("http_address", cfg.HTTPAddress))
	select {
	case <-ctx.Done():
	case <-engine.Failed():
		err = errors.New("voice durable state unavailable")
		cancel()
	case err = <-failures:
		cancel()
	}
	engine.Stop()
	done := make(chan struct{})
	go func() { rpc.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		rpc.Stop()
		<-done
	}
	engine.Wait()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = httpServer.Shutdown(shutdown)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

var trace = regexp.MustCompile(`^00-([0-9a-f]{32})-[0-9a-f]{16}-0[01]$`)

func traceID(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("traceparent")
	if len(values) != 1 {
		return ""
	}
	parts := trace.FindStringSubmatch(values[0])
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}
func requestLevel(err error) slog.Level {
	if err == nil || status.Code(err) == codes.Canceled {
		return slog.LevelInfo
	}
	return slog.LevelWarn
}
