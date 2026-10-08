package application

import (
	"context"
	"fmt"
	"mime"
	"strings"
	"sync"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

const maxAudioObjectBytes = 64 * 1024 * 1024

type WorkerConfig struct {
	Concurrency                           int
	PollInterval, VoiceTimeout, S3Timeout time.Duration
	Bucket                                string
}

type Worker struct {
	queue  Queue
	store  Storage
	synth  Synthesizer
	config WorkerConfig
}

func NewWorker(queue Queue, store Storage, synth Synthesizer, config WorkerConfig) *Worker {
	return &Worker{queue: queue, store: store, synth: synth, config: config}
}

func RetryDelay(attempt int) time.Duration {
	attempt = max(1, min(attempt, 5))
	return min(5*time.Second*time.Duration(1<<(attempt-1)), 60*time.Second)
}

// ProcessOne claims and handles at most one asset. Errors are returned after a
// safe retry transition; cancellation leaves the lease for recovery on restart.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	if err := w.validate(); err != nil {
		return false, err
	}
	token, err := security.ID("audio-claim")
	if err != nil {
		return false, fmt.Errorf("create task audio claim token: %w", err)
	}
	lease := w.config.VoiceTimeout + 3*w.config.S3Timeout + 30*time.Second
	claim, ok, err := w.queue.Claim(ctx, token, lease)
	if err != nil || !ok {
		return ok, err
	}
	if err := w.processClaim(ctx, claim); err != nil {
		return true, err
	}
	return true, nil
}

func (w *Worker) processClaim(ctx context.Context, claim Claim) error {
	statCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
	info, found, err := w.store.Stat(statCtx, w.config.Bucket, claim.Asset.ObjectKey)
	cancel()
	if err != nil {
		if isCancelled(ctx, err) {
			return err
		}
		return w.fail(ctx, claim, "storage_head_failed", err)
	}
	if found && validObject(info, claim.Asset.ID) {
		return w.complete(ctx, claim)
	}
	current, err := w.queue.IsCurrent(ctx, claim)
	if err != nil {
		return w.fail(ctx, claim, "task_current_check_failed", err)
	}
	if !current {
		if _, err := w.queue.Cancel(ctx, claim); err != nil {
			return fmt.Errorf("cancel stale task audio claim: %w", err)
		}
		return nil
	}

	voiceCtx, cancel := context.WithTimeout(ctx, w.config.VoiceTimeout)
	data, err := w.synth.Synthesize(voiceCtx, claim.Asset.Instruction)
	cancel()
	if err != nil {
		if isCancelled(ctx, err) {
			return err
		}
		return w.fail(ctx, claim, "synthesis_failed", err)
	}
	if len(data) == 0 || len(data) > maxAudioObjectBytes || !audio.ValidWAV(data) {
		return w.fail(ctx, claim, "invalid_wav", fmt.Errorf("synthesizer returned invalid WAV"))
	}

	putCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
	err = w.store.Put(putCtx, w.config.Bucket, claim.Asset.ObjectKey, claim.Asset.ID, data)
	cancel()
	if err != nil {
		if isCancelled(ctx, err) {
			return err
		}
		return w.fail(ctx, claim, "storage_put_failed", err)
	}
	return w.complete(ctx, claim)
}

func validObject(info ObjectInfo, assetID string) bool {
	if info.Size < 1 || info.Size > maxAudioObjectBytes || info.AssetID != assetID {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(info.ContentType)
	return err == nil && strings.EqualFold(mediaType, "audio/wav")
}

func (w *Worker) complete(ctx context.Context, claim Claim) error {
	storageURI := "s3://" + w.config.Bucket + "/" + claim.Asset.ObjectKey
	audioURL := "/task-audio/" + claim.Asset.ID + "/file"
	_, err := w.queue.Complete(ctx, claim, w.config.Bucket, storageURI, audioURL)
	if err != nil {
		// Leaving processing state lets the next lease recover by checking HEAD.
		return fmt.Errorf("complete task audio: %w", err)
	}
	return nil
}

func (w *Worker) fail(ctx context.Context, claim Claim, code string, cause error) error {
	if ctx.Err() != nil {
		return cause
	}
	_, err := w.queue.Fail(ctx, claim, code, RetryDelay(claim.Asset.Attempts))
	if err != nil {
		return fmt.Errorf("record task audio failure (%s): %w", code, err)
	}
	return fmt.Errorf("task audio %s: %w", code, cause)
}

func isCancelled(ctx context.Context, _ error) bool {
	return ctx.Err() != nil
}

// Run provisions the bucket before claiming jobs, then owns a fixed number of
// lanes until the caller cancels the context.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.validate(); err != nil {
		return err
	}
	backoff := time.Second
	for {
		ensureCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
		err := w.store.EnsureBucket(ensureCtx)
		cancel()
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return nil
		}
		if err := wait(ctx, backoff); err != nil {
			return nil
		}
		backoff = min(backoff*2, 30*time.Second)
	}

	var workers sync.WaitGroup
	workers.Add(w.config.Concurrency)
	for range w.config.Concurrency {
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				_, err := w.ProcessOne(ctx)
				if err != nil && ctx.Err() != nil {
					return
				}
				if err != nil {
					if wait(ctx, w.config.PollInterval) != nil {
						return
					}
					continue
				}
				// Whether one item was processed or the queue was empty, bound idle polling.
				if wait(ctx, w.config.PollInterval) != nil {
					return
				}
			}
		}()
	}
	workers.Wait()
	return nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (w *Worker) validate() error {
	if w.queue == nil || w.store == nil || w.synth == nil {
		return fmt.Errorf("task audio queue, storage and synthesizer are required")
	}
	if w.config.Concurrency <= 0 || w.config.PollInterval <= 0 || w.config.VoiceTimeout <= 0 || w.config.S3Timeout <= 0 || strings.TrimSpace(w.config.Bucket) == "" {
		return fmt.Errorf("task audio worker configuration is invalid")
	}
	return nil
}
