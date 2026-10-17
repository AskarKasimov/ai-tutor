package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
)

type OCRJob struct {
	MaterialID string
	Token      string
	Attempts   int
}

type OCRQueue interface {
	ClaimOCR(context.Context, string, time.Time) (OCRJob, error)
	Get(context.Context, string) (material.Material, error)
	FinishOCR(context.Context, OCRJob, string, string, json.RawMessage, string, *string, time.Time) (bool, error)
	FailOCR(context.Context, OCRJob, string, time.Time) error
}

type DetailedOCRResult struct {
	Markdown string
	Report   json.RawMessage
}

type OCRWorker struct {
	queue      OCRQueue
	storage    S3Storage
	ocr        OCRParser
	translator Translator
}

func NewOCRWorker(queue OCRQueue, storage S3Storage, ocr OCRParser, translator Translator) *OCRWorker {
	return &OCRWorker{queue: queue, storage: storage, ocr: ocr, translator: translator}
}

func (w *OCRWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		worked, err := w.ProcessOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("Material OCR: %v", err)
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

func (w *OCRWorker) ProcessOne(ctx context.Context) (bool, error) {
	allowPDF := true
	if readiness, ok := w.ocr.(interface{ Ready(context.Context) error }); ok {
		if err := readiness.Ready(ctx); err != nil {
			allowPDF = false // Keep text materials moving while shared PDF models load.
		}
	}
	var job OCRJob
	var err error
	if queue, ok := w.queue.(interface {
		ClaimOCRReady(context.Context, string, time.Time, bool) (OCRJob, error)
	}); ok {
		job, err = queue.ClaimOCRReady(ctx, generateID("ocr_"), time.Now(), allowPDF)
	} else if allowPDF {
		job, err = w.queue.ClaimOCR(ctx, generateID("ocr_"), time.Now())
	}
	if err != nil {
		return false, err
	}
	if job.MaterialID == "" {
		return false, nil
	}
	callbacks := leaseCallbacks{}
	if owner, ok := w.queue.(interface {
		RenewOCR(context.Context, OCRJob, time.Time) (bool, error)
		ReleaseOCR(context.Context, OCRJob, time.Time) error
	}); ok {
		callbacks.renew = func(ctx context.Context, now time.Time) (bool, error) { return owner.RenewOCR(ctx, job, now) }
		callbacks.release = func(ctx context.Context, now time.Time) error { return owner.ReleaseOCR(ctx, job, now) }
	}
	if err := runLeased(ctx, callbacks, func(taskCtx context.Context) error { return w.process(taskCtx, job) }); err != nil {
		if ctx.Err() != nil || errors.Is(err, errLeaseLost) {
			return true, nil
		}
		retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if failErr := w.queue.FailOCR(retryCtx, job, err.Error(), time.Now()); failErr != nil {
			return true, fmt.Errorf("OCR failed: %v; record failure: %w", err, failErr)
		}
		return true, err
	}
	return true, nil
}

func (w *OCRWorker) process(ctx context.Context, job OCRJob) error {
	mat, err := w.queue.Get(ctx, job.MaterialID)
	if err != nil {
		return fmt.Errorf("read material %s: %w", job.MaterialID, err)
	}
	raw, err := w.storage.Get(ctx, mat.S3RawKey)
	if err != nil {
		return fmt.Errorf("read raw material %s: %w", job.MaterialID, err)
	}
	var markdown string
	var ocrReport json.RawMessage
	var ocrErr error
	if jobParser, ok := w.ocr.(interface {
		ConvertJobDetailed(context.Context, string, string, []byte) (string, json.RawMessage, error)
	}); ok {
		markdown, ocrReport, ocrErr = jobParser.ConvertJobDetailed(ctx, mat.ID, mat.Filename, raw)
	} else if jobParser, ok := w.ocr.(interface {
		ConvertJob(context.Context, string, string, []byte) (string, error)
	}); ok {
		markdown, ocrErr = jobParser.ConvertJob(ctx, mat.ID, mat.Filename, raw)
	} else {
		markdown, ocrErr = w.ocr.Convert(ctx, mat.Filename, raw)
	}
	if strings.TrimSpace(markdown) == "" {
		if ocrErr != nil {
			return fmt.Errorf("OCR produced no text: %w", ocrErr)
		}
		return fmt.Errorf("OCR produced no text")
	}

	warnings := make([]string, 0, 2)
	if ocrErr != nil {
		warnings = append(warnings, "Предупреждение OCR: "+ocrErr.Error())
	}
	base := fmt.Sprintf("subjects/%s", mat.SubjectID)
	ocrKey := fmt.Sprintf("%s/ocr/%s/%s.md", base, mat.ID, job.Token)
	if err := w.storage.Put(ctx, ocrKey, "text/markdown; charset=utf-8", []byte(markdown)); err != nil {
		return fmt.Errorf("store raw OCR: %w", err)
	}
	finalText := markdown
	translated := false
	if w.translator != nil {
		if queue, ok := w.queue.(interface {
			SetOCRStage(context.Context, OCRJob, string, time.Time) (bool, error)
		}); ok {
			owned, stageErr := queue.SetOCRStage(ctx, job, "translation", time.Now())
			if stageErr != nil || !owned {
				if stageErr != nil {
					return fmt.Errorf("update OCR stage: %w", stageErr)
				}
				return errLeaseLost
			}
		}
		translatedText, translateErr := w.translator.Translate(ctx, markdown)
		switch {
		case translateErr != nil:
			warnings = append(warnings, "Перевод не завершён; используется исходный OCR: "+translateErr.Error())
		case strings.TrimSpace(translatedText) != "" && translatedText != markdown:
			finalText, translated = translatedText, true
		}
	}

	key := fmt.Sprintf("%s/parsed/%s/%s.md", base, mat.ID, job.Token)
	if translated {
		key = fmt.Sprintf("%s/translated/%s/%s.md", base, mat.ID, job.Token)
	}
	if err := w.storage.Put(ctx, key, "text/markdown; charset=utf-8", []byte(finalText)); err != nil {
		if !translated {
			return fmt.Errorf("store parsed material: %w", err)
		}
		warnings = append(warnings, "Не удалось сохранить перевод; используется исходный OCR.")
		finalText = markdown
		key = fmt.Sprintf("%s/parsed/%s/%s.md", base, mat.ID, job.Token)
		if err := w.storage.Put(ctx, key, "text/markdown; charset=utf-8", []byte(finalText)); err != nil {
			return fmt.Errorf("store OCR material: %w", err)
		}
	}

	var warning *string
	if len(ocrReport) == 0 {
		ocrReport = json.RawMessage(`{"engine":"unknown","warnings":[]}`)
	}
	if len(warnings) > 0 {
		var report map[string]any
		if json.Unmarshal(ocrReport, &report) == nil {
			report["pipeline_warnings"] = warnings
			ocrReport, _ = json.Marshal(report)
		}
	}
	if len(warnings) > 0 {
		joined := strings.Join(warnings, "; ")
		warning = &joined
	}
	accepted, err := w.queue.FinishOCR(ctx, job, ocrKey, key, ocrReport, finalText, warning, time.Now())
	if err != nil {
		return fmt.Errorf("save OCR result: %w", err)
	}
	if !accepted {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = w.storage.Delete(cleanupCtx, key)
		_ = w.storage.Delete(cleanupCtx, ocrKey)
	}
	return nil
}
