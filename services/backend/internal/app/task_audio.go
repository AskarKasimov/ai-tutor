package app

import (
	"context"
	"fmt"
)

// RunAudioWorker starts the app-owned background worker. Handler construction
// deliberately does not start background services.
func (a *App) RunAudioWorker(ctx context.Context) error {
	if a.audioWorker == nil {
		return fmt.Errorf("task audio worker is not configured")
	}
	return a.audioWorker.Run(ctx)
}
