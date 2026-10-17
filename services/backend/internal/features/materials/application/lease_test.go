package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunLeasedReleasesOwnTokenOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var released atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- runLeased(ctx, leaseCallbacks{
			renew:   func(context.Context, time.Time) (bool, error) { return true, nil },
			release: func(context.Context, time.Time) error { released.Store(true); return nil },
		}, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runLeased error = %v, want context canceled", err)
	}
	if !released.Load() {
		t.Fatal("job lease was not released on shutdown")
	}
}
