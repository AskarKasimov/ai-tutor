package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func serveWithAudioWorker(ctx context.Context, server *http.Server, runWorker func(context.Context) error) error {
	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- runWorker(workerCtx) }()

	serverDone := make(chan error, 1)
	go func() { serverDone <- server.ListenAndServe() }()

	stopWorker := func() error {
		cancelWorker()
		return <-workerDone
	}
	select {
	case workerErr := <-workerDone:
		cancelWorker()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		if workerErr != nil {
			return fmt.Errorf("background workers: %w", workerErr)
		}
		return nil
	case serverErr := <-serverDone:
		workerErr := stopWorker()
		if errors.Is(serverErr, http.ErrServerClosed) {
			return workerErr
		}
		if serverErr != nil && workerErr != nil {
			return errors.Join(serverErr, fmt.Errorf("audio worker: %w", workerErr))
		}
		return serverErr
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		shutdownErr := server.Shutdown(shutdownCtx)
		cancel()
		workerErr := stopWorker()
		if shutdownErr != nil {
			return shutdownErr
		}
		return workerErr
	}
}
