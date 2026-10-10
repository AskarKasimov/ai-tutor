package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"sync"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audioasset"
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
	slots  chan struct{}
}

func NewWorker(queue Queue, store Storage, synth Synthesizer, config WorkerConfig) *Worker {
	return &Worker{queue: queue, store: store, synth: synth, config: config, slots: make(chan struct{}, max(1, config.Concurrency))}
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
	if err := w.acquire(ctx); err != nil {
		return false, err
	}
	defer w.release()
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

// Regenerate synchronously repairs one ready or failed asset while sharing the worker's
// concurrency bound. Only an atomically claimed, current-map asset can run.
func (w *Worker) Regenerate(ctx context.Context, assetID string) (audioasset.Asset, error) {
	if err := w.validate(); err != nil {
		return audioasset.Asset{}, err
	}
	if assetID == "" {
		return audioasset.Asset{}, audioasset.ErrNotRepairable
	}
	budget := w.config.VoiceTimeout + 5*w.config.S3Timeout + 30*time.Second
	operationCtx, cancelOperation := context.WithTimeout(ctx, budget)
	defer cancelOperation()
	if err := w.acquire(operationCtx); err != nil {
		return audioasset.Asset{}, err
	}
	defer w.release()
	if err := operationCtx.Err(); err != nil {
		return audioasset.Asset{}, err
	}
	token, err := security.ID("audio-repair")
	if err != nil {
		return audioasset.Asset{}, fmt.Errorf("create task audio repair token: %w", err)
	}
	lease := budget
	claim, ok, err := w.queue.ClaimRepair(operationCtx, assetID, token, lease)
	if err != nil {
		return audioasset.Asset{}, fmt.Errorf("claim task audio for repair: %w", err)
	}
	if !ok {
		return audioasset.Asset{}, audioasset.ErrNotRepairable
	}
	bucket, err := w.processRepairClaim(operationCtx, claim)
	if err != nil {
		return audioasset.Asset{}, err
	}
	uri := "s3://" + bucket + "/" + claim.Asset.ObjectKey
	url := "/task-audio/" + claim.Asset.ID + "/file"
	claim.Asset.Bucket, claim.Asset.StorageURI, claim.Asset.AudioURL = &bucket, &uri, &url
	claim.Asset.Status = audioasset.Ready
	return claim.Asset, nil
}

func (w *Worker) processRepairClaim(ctx context.Context, claim Claim) (string, error) {
	readBucket := w.config.Bucket
	if claim.Asset.Bucket != nil && *claim.Asset.Bucket != "" {
		readBucket = *claim.Asset.Bucket
	}
	if readBucket == w.config.Bucket {
		if err := w.ensureRepairBucket(ctx, claim); err != nil {
			return "", err
		}
	}
	statCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
	info, found, err := w.store.Stat(statCtx, readBucket, claim.Asset.ObjectKey)
	cancel()
	if err != nil {
		if readBucket == w.config.Bucket || !errors.Is(err, ErrBucketNotFound) {
			return "", w.repairFailure(ctx, claim, "storage_head_failed", audioasset.ErrStorageUnavailable, err)
		}
		found = false
	}
	if !found && readBucket != w.config.Bucket {
		if checker, ok := w.store.(BucketChecker); ok {
			checkCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
			exists, checkErr := checker.BucketExists(checkCtx, readBucket)
			cancel()
			if checkErr != nil {
				return "", w.repairFailure(ctx, claim, "storage_bucket_check_failed", audioasset.ErrStorageUnavailable, checkErr)
			}
			if !exists {
				readBucket = ""
			}
		}
	}
	if found && validObject(info, claim.Asset.ID) {
		openCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
		body, getInfo, openErr := w.store.Open(openCtx, readBucket, claim.Asset.ObjectKey)
		if openErr != nil && !errors.Is(openErr, ErrObjectNotFound) && !errors.Is(openErr, ErrBucketNotFound) {
			cancel()
			return "", w.repairFailure(ctx, claim, "storage_get_failed", audioasset.ErrStorageUnavailable, openErr)
		}
		if body != nil {
			data, readErr := io.ReadAll(io.LimitReader(body, maxAudioObjectBytes+1))
			closeErr := body.Close()
			cancel()
			if readErr != nil || closeErr != nil {
				cause := errors.Join(readErr, closeErr)
				return "", w.repairFailure(ctx, claim, "storage_read_failed", audioasset.ErrStorageUnavailable, cause)
			}
			if validObject(getInfo, claim.Asset.ID) && int64(len(data)) == getInfo.Size && audio.ValidWAV(data) {
				return readBucket, w.completeRepair(ctx, claim, readBucket)
			}
		} else {
			cancel()
		}
		if errors.Is(openErr, ErrBucketNotFound) {
			found = false
		}
	}
	current, err := w.queue.IsCurrent(ctx, claim)
	if err != nil {
		return "", w.repairFailure(ctx, claim, "task_current_check_failed", audioasset.ErrStorageUnavailable, err)
	}
	if !current {
		_, cancelErr := w.queue.Cancel(ctx, claim)
		if cancelErr != nil {
			return "", fmt.Errorf("cancel stale task audio repair: %w", cancelErr)
		}
		return "", audioasset.ErrNotRepairable
	}
	if readBucket != w.config.Bucket {
		if err := w.ensureRepairBucket(ctx, claim); err != nil {
			return "", err
		}
	}
	voiceCtx, cancel := context.WithTimeout(ctx, w.config.VoiceTimeout)
	data, err := w.synth.Synthesize(voiceCtx, claim.Asset.Instruction)
	cancel()
	if err != nil {
		return "", w.repairFailure(ctx, claim, "synthesis_failed", audioasset.ErrGenerationFailed, err)
	}
	if len(data) == 0 || len(data) > maxAudioObjectBytes || !audio.ValidWAV(data) {
		return "", w.repairFailure(ctx, claim, "invalid_wav", audioasset.ErrGenerationFailed, errors.New("synthesizer returned invalid WAV"))
	}
	putCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
	err = w.store.Put(putCtx, w.config.Bucket, claim.Asset.ObjectKey, claim.Asset.ID, data)
	cancel()
	if err != nil {
		return "", w.repairFailure(ctx, claim, "storage_put_failed", audioasset.ErrStorageUnavailable, err)
	}
	return w.config.Bucket, w.completeRepair(ctx, claim, w.config.Bucket)
}

func (w *Worker) ensureRepairBucket(ctx context.Context, claim Claim) error {
	ensureCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
	err := w.store.EnsureBucket(ensureCtx)
	cancel()
	if err != nil {
		return w.repairFailure(ctx, claim, "storage_bucket_failed", audioasset.ErrStorageUnavailable, err)
	}
	return nil
}

func (w *Worker) completeRepair(ctx context.Context, claim Claim, bucket string) error {
	uri := "s3://" + bucket + "/" + claim.Asset.ObjectKey
	url := "/task-audio/" + claim.Asset.ID + "/file"
	ok, err := w.queue.Complete(ctx, claim, bucket, uri, url)
	if err != nil {
		return fmt.Errorf("complete task audio repair: %w", err)
	}
	if !ok {
		return audioasset.ErrNotRepairable
	}
	return nil
}

func (w *Worker) repairFailure(ctx context.Context, claim Claim, code string, kind, cause error) error {
	if ctx.Err() == nil {
		if _, err := w.queue.Fail(ctx, claim, code, RetryDelay(claim.Asset.Attempts)); err != nil {
			return fmt.Errorf("record task audio repair failure (%s): %w", code, err)
		}
	}
	return fmt.Errorf("%w: %v", kind, cause)
}

func (w *Worker) acquire(ctx context.Context) error {
	select {
	case w.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) release() { <-w.slots }

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

	data, reused := w.copyReady(ctx, claim)
	if !reused {
		voiceCtx, cancel := context.WithTimeout(ctx, w.config.VoiceTimeout)
		data, err = w.synth.Synthesize(voiceCtx, claim.Asset.Instruction)
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

// copyReady reads a ready recording of the same instruction so a re-imported
// map does not wait for TTS again. Every failure falls back to synthesis; the
// copy is stored under the claimed asset's own key, keeping assets isolated.
func (w *Worker) copyReady(ctx context.Context, claim Claim) ([]byte, bool) {
	source, ok, err := w.queue.FindReady(ctx, claim.Asset.Instruction, claim.Asset.ID)
	if err != nil || !ok || source.Bucket == nil || *source.Bucket == "" || source.ObjectKey == "" {
		return nil, false
	}
	openCtx, cancel := context.WithTimeout(ctx, w.config.S3Timeout)
	defer cancel()
	body, info, err := w.store.Open(openCtx, *source.Bucket, source.ObjectKey)
	if err != nil || body == nil {
		return nil, false
	}
	data, readErr := io.ReadAll(io.LimitReader(body, maxAudioObjectBytes+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || !validObject(info, source.ID) ||
		int64(len(data)) != info.Size || !audio.ValidWAV(data) {
		return nil, false
	}
	return data, true
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
