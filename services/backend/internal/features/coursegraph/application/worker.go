package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/material"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/user"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/security"
)

type Repository interface {
	Claim(context.Context, string, time.Time) (coursegraph.Job, error)
	Finish(context.Context, coursegraph.Job, json.RawMessage, time.Time) (bool, error)
	Fail(context.Context, coursegraph.Job, string, time.Time) error
	Read(context.Context, string) (coursegraph.Snapshot, error)
}
type Materials interface {
	List(context.Context, string) ([]material.Material, error)
}
type Storage interface {
	Get(context.Context, string) ([]byte, error)
}
type Builder interface {
	Build(context.Context, []coursegraph.Material) (json.RawMessage, error)
}

type Worker struct {
	repo      Repository
	materials Materials
	storage   Storage
	builder   Builder
}

func New(repo Repository, materials Materials, storage Storage, builder Builder) *Worker {
	return &Worker{repo: repo, materials: materials, storage: storage, builder: builder}
}

func (w *Worker) Read(ctx context.Context, actor user.User, subjectID string) (coursegraph.Snapshot, error) {
	if actor.Role != user.Admin {
		return coursegraph.Snapshot{}, fault.New(fault.Forbidden, "FORBIDDEN", "Доступно только admin.")
	}
	return w.repo.Read(ctx, subjectID)
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		worked, err := w.ProcessOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("Course graph: %v", err)
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

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	token, err := security.Token()
	if err != nil {
		return false, err
	}
	job, err := w.repo.Claim(ctx, token, time.Now())
	if err != nil || job.SubjectID == "" {
		return false, err
	}
	taskCtx, cancel := context.WithTimeout(ctx, 41*time.Minute)
	defer cancel()
	err = w.process(taskCtx, job)
	if err != nil {
		failureCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		if failure := w.repo.Fail(failureCtx, job, "Не удалось построить граф; предыдущий граф сохранён.", time.Now()); failure != nil {
			return true, fmt.Errorf("%v; record failure: %w", err, failure)
		}
	}
	return true, err
}

func (w *Worker) process(ctx context.Context, job coursegraph.Job) error {
	if job.Attempts > 3 {
		return fmt.Errorf("graph attempt limit reached")
	}
	items, err := w.materials.List(ctx, job.SubjectID)
	if err != nil {
		return err
	}
	inputs := make([]coursegraph.Material, 0, len(items))
	for _, m := range items {
		if m.Cleaning == nil || m.Cleaning.Status != "ready" {
			return fmt.Errorf("material %s is not ready", m.ID)
		}
		source, err := w.storage.Get(ctx, m.S3ParsedKey)
		if err != nil {
			return err
		}
		report, err := w.storage.Get(ctx, m.Cleaning.S3ReportKey)
		if err != nil {
			return err
		}
		var cached struct {
			Topics        json.RawMessage `json:"topics"`
			TopicsVersion string          `json:"topics_version"`
		}
		if err = json.Unmarshal(report, &cached); err != nil {
			return err
		}
		input := coursegraph.Material{ID: m.ID, Markdown: string(source)}
		if cached.TopicsVersion == "topics-v2" {
			input.Topics = cached.Topics
		}
		inputs = append(inputs, input)
	}
	if len(inputs) == 0 {
		return fmt.Errorf("empty course")
	}
	graph, err := w.builder.Build(ctx, inputs)
	if err != nil {
		return err
	}
	var shape struct {
		Version       string            `json:"version"`
		Topics        []json.RawMessage `json:"topics"`
		Prerequisites []json.RawMessage `json:"prerequisites"`
		Quality       struct {
			Acyclic bool `json:"acyclic"`
		} `json:"quality"`
	}
	if json.Unmarshal(graph, &shape) != nil || shape.Version != "course-graph-v1" || !shape.Quality.Acyclic || shape.Topics == nil || shape.Prerequisites == nil {
		return fmt.Errorf("invalid graph result")
	}
	_, err = w.repo.Finish(ctx, job, graph, time.Now())
	return err
}
