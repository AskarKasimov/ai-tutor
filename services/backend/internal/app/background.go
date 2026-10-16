package app

import (
	"context"
	"errors"
	"net"
	"time"

	llmmodel "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/llm/infrastructure/modelapi"
	llmgrpc "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/llm/transport/grpc"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/llmpb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// RunBackground owns lifecycle of durable workers and the internal model gateway.
func (a *App) RunBackground(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error { return a.RunAudioWorker(ctx) })
	group.Go(func() error { return a.cleaningWorker.Run(ctx) })
	group.Go(func() error {
		address := a.cfg.LLMGRPCAddress
		if address == "" {
			address = ":8003"
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}
		endpoints := a.cfg.LLMBaseURLs
		if len(endpoints) == 0 {
			endpoints = []string{a.cfg.AssessmentBaseURL}
		}
		model := a.cfg.LLMModel
		if model == "" {
			model = a.cfg.AssessmentModel
		}
		server := grpc.NewServer(grpc.MaxRecvMsgSize(128*1024), grpc.MaxConcurrentStreams(8))
		llmpb.RegisterLLMServer(server, &llmgrpc.Server{Client: llmmodel.New(a.client, endpoints, model, a.cfg.LLMAPIKey, 110*time.Second)})
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				server.Stop()
			case <-done:
			}
		}()
		err = server.Serve(listener)
		close(done)
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	})
	return group.Wait()
}
