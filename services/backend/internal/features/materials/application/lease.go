package application

import (
	"context"
	"errors"
	"time"
)

var errLeaseLost = errors.New("material job lease lost")

type leaseCallbacks struct {
	renew   func(context.Context, time.Time) (bool, error)
	release func(context.Context, time.Time) error
}

// runLeased keeps long processing jobs owned without imposing a total runtime limit.
func runLeased(ctx context.Context, callbacks leaseCallbacks, process func(context.Context) error) error {
	if callbacks.renew == nil {
		return process(ctx)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- process(workCtx) }()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil {
				if callbacks.release != nil {
					releaseCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
					_ = callbacks.release(releaseCtx, time.Now())
					stop()
				}
				return ctx.Err()
			}
			return err
		case <-ctx.Done():
			cancel()
			<-done
			if callbacks.release != nil {
				releaseCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				_ = callbacks.release(releaseCtx, time.Now())
				stop()
			}
			return ctx.Err()
		case <-ticker.C:
			heartbeatCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			owned, err := callbacks.renew(heartbeatCtx, time.Now())
			stop()
			if err != nil || !owned {
				cancel()
				<-done
				if err != nil {
					return err
				}
				return errLeaseLost
			}
		}
	}
}
