package app

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
)

type countingAudioWorker struct{ runs atomic.Int32 }

func (w *countingAudioWorker) Run(context.Context) error { w.runs.Add(1); return nil }
func (w *countingAudioWorker) ProcessOne(context.Context) (bool, error) {
	return false, nil
}
func (w *countingAudioWorker) Regenerate(context.Context, string) (audioasset.Asset, error) {
	return audioasset.Asset{}, audioasset.ErrNotRepairable
}

func TestHandlerDoesNotStartAudioWorker(t *testing.T) {
	f := newFixture(t)
	worker := &countingAudioWorker{}
	f.app.audioWorker = worker
	_ = f.app.Handler()
	_ = f.app.Handler()
	if got := worker.runs.Load(); got != 0 {
		t.Fatalf("Handler started audio worker %d times", got)
	}
	if err := f.app.RunAudioWorker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := worker.runs.Load(); got != 1 {
		t.Fatalf("explicit worker start count=%d", got)
	}
}
