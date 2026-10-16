package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
)

type CleaningResult struct {
	Markdown string          `json:"markdown"`
	Report   json.RawMessage `json:"report"`
}
type Cleaner interface {
	Clean(context.Context, string) (CleaningResult, error)
}
type CleaningQueue interface {
	ClaimCleaning(context.Context, string, time.Time) (material.CleaningJob, error)
	Get(context.Context, string) (material.Material, error)
	FinishCleaning(context.Context, material.CleaningJob, CleaningResult, string, string, time.Time) (bool, error)
	FailCleaning(context.Context, material.CleaningJob, string, time.Time) error
}

type CleaningWorker struct {
	queue   CleaningQueue
	storage S3Storage
	cleaner Cleaner
}

func NewCleaningWorker(queue CleaningQueue, storage S3Storage, cleaner Cleaner) *CleaningWorker {
	return &CleaningWorker{queue: queue, storage: storage, cleaner: cleaner}
}
func (w *CleaningWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		worked, err := w.ProcessOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("Material cleaning: %v", err)
		}
		if worked && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *CleaningWorker) ProcessOne(ctx context.Context) (bool, error) {
	job, err := w.queue.ClaimCleaning(ctx, generateID("clean_"), time.Now())
	if err != nil {
		return false, err
	}
	if job.MaterialID == "" {
		return false, nil
	}
	taskCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	err = w.process(taskCtx, job)
	if err != nil {
		retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer retryCancel()
		if failErr := w.queue.FailCleaning(retryCtx, job, "Не удалось очистить материал; исходный текст сохранён.", time.Now()); failErr != nil {
			return true, fmt.Errorf("cleaning failed: %v; record failure: %w", err, failErr)
		}
	}
	return true, err
}

func (w *CleaningWorker) process(ctx context.Context, job material.CleaningJob) error {
	m, err := w.queue.Get(ctx, job.MaterialID)
	if err != nil {
		return err
	}
	if job.Attempts > 3 {
		return fmt.Errorf("cleaning attempt limit reached")
	}
	source, err := w.storage.Get(ctx, m.S3ParsedKey)
	if err != nil {
		return fmt.Errorf("read parsed material: %w", err)
	}
	if len(source) == 0 || len(source) > 8*1024*1024 {
		return fmt.Errorf("invalid parsed material size")
	}
	result, err := w.cleaner.Clean(ctx, string(source))
	if err != nil {
		return err
	}
	if !validCleaningResult(string(source), result) {
		return fmt.Errorf("invalid cleaning result")
	}
	// Attempt-specific objects prevent an expired worker from overwriting its successor.
	prefix := fmt.Sprintf("subjects/%s/cleaned/%s/%s", m.SubjectID, m.ID, job.Token)
	textKey, reportKey := prefix+".md", prefix+".json"
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		for _, key := range []string{textKey, reportKey} {
			_ = w.storage.Delete(cleanupCtx, key)
		}
	}
	if err := w.storage.Put(ctx, textKey, "text/markdown; charset=utf-8", []byte(result.Markdown)); err != nil {
		cleanup()
		return err
	}
	if err := w.storage.Put(ctx, reportKey, "application/json", result.Report); err != nil {
		cleanup()
		return err
	}
	accepted, err := w.queue.FinishCleaning(ctx, job, result, textKey, reportKey, time.Now())
	if err != nil {
		return err
	} // Ambiguous DB commit: keep objects; a committed result must remain readable.
	if !accepted {
		cleanup()
	}
	return nil
}
