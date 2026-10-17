package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
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
	Request(context.Context, string, time.Time) (bool, error)
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

// Voices synthesizes prepared questions in the background; Prepare queues
// them without waiting.
type Voices interface {
	Prepare(texts []string) int
}

// DefaultGraph is the graph Geek examines a course by before its first build.
type DefaultGraph interface {
	Current(context.Context, string) (exam.Graph, bool, error)
}

type Worker struct {
	repo      Repository
	materials Materials
	storage   Storage
	builder   Builder
	voices    Voices
	fallback  DefaultGraph
}

// New builds the graph worker; voices may be nil, then questions are voiced
// only when a student first hears them. fallback may be nil, then a course
// without a build has no graph.
func New(repo Repository, materials Materials, storage Storage, builder Builder, voices Voices, fallback DefaultGraph) *Worker {
	return &Worker{repo: repo, materials: materials, storage: storage, builder: builder, voices: voices, fallback: fallback}
}

// TopicQuestions are the prepared questions of a built graph, one per topic.
func TopicQuestions(graph json.RawMessage) []string {
	var shape struct {
		Topics []struct {
			Question string `json:"question"`
		} `json:"topics"`
	}
	out := []string{}
	if json.Unmarshal(graph, &shape) != nil {
		return out
	}
	for _, t := range shape.Topics {
		if q := strings.TrimSpace(t.Question); q != "" {
			out = append(out, q)
		}
	}
	return out
}

func (w *Worker) Read(ctx context.Context, actor user.User, subjectID string) (coursegraph.Snapshot, error) {
	if actor.Role != user.Admin {
		return coursegraph.Snapshot{}, fault.New(fault.Forbidden, "FORBIDDEN", "Доступно только admin.")
	}
	return w.snapshot(ctx, subjectID)
}

// snapshot shows the admin the graph Geek uses: the build while Geek reads
// it, otherwise the default graph in the shape of a build. The default graph
// only enriches the answer, so its failure leaves the snapshot without a
// graph instead of failing a read or a build request already accepted.
func (w *Worker) snapshot(ctx context.Context, subjectID string) (coursegraph.Snapshot, error) {
	snapshot, err := w.repo.Read(ctx, subjectID)
	if err != nil {
		return coursegraph.Snapshot{}, err
	}
	snapshot.GraphSource = coursegraph.SourceNone
	if len(snapshot.Graph) > 0 && string(snapshot.Graph) != "null" {
		// Geek reads a published graph only while the course has materials
		// (ReadPublishedCourseGraph); without them it falls back as well.
		items, err := w.materials.List(ctx, subjectID)
		if err != nil {
			return coursegraph.Snapshot{}, err
		}
		if len(items) > 0 {
			snapshot.GraphSource = coursegraph.SourceBuilt
			return snapshot, nil
		}
		snapshot.Graph = nil
	}
	if w.fallback == nil {
		return snapshot, nil
	}
	g, ok, err := w.fallback.Current(ctx, subjectID)
	if err == nil && ok {
		var shape json.RawMessage
		if shape, err = BuildShape(g); err == nil {
			snapshot.Graph, snapshot.GraphSource = shape, coursegraph.SourceFixture
		}
	}
	if err != nil {
		log.Printf("course graph %s: the default graph is not shown: %v", subjectID, err)
	}
	return snapshot, nil
}

// BuildShape writes an examined graph as course-graph writes a build:
// topics, prerequisite edges from the prerequisite to the topic, and an empty
// hierarchy, since the examined graph has no sections.
func BuildShape(g exam.Graph) (json.RawMessage, error) {
	type topic struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Question string   `json:"question,omitempty"`
		Text     string   `json:"text,omitempty"`
		Provides []string `json:"provides"`
		Requires []string `json:"requires"`
	}
	type edge struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	shape := struct {
		Version       string  `json:"version"`
		Topics        []topic `json:"topics"`
		Prerequisites []edge  `json:"prerequisites"`
		Hierarchy     []edge  `json:"hierarchy"`
	}{Version: g.Version, Topics: []topic{}, Prerequisites: []edge{}, Hierarchy: []edge{}}
	for _, n := range g.Nodes {
		t := topic{ID: n.ID, Title: n.Title, Text: n.Content, Provides: n.Outcomes, Requires: []string{}}
		if len(n.Questions) > 0 {
			t.Question = n.Questions[0]
		}
		if t.Provides == nil {
			t.Provides = []string{}
		}
		shape.Topics = append(shape.Topics, t)
		for _, p := range n.Prerequisites {
			shape.Prerequisites = append(shape.Prerequisites, edge{Source: p, Target: n.ID})
		}
	}
	return json.Marshal(shape)
}

// Request starts a build of the course graph from the current materials (the
// admin's "build graph" button).
func (w *Worker) Request(ctx context.Context, actor user.User, subjectID string) (coursegraph.Snapshot, error) {
	if actor.Role != user.Admin {
		return coursegraph.Snapshot{}, fault.New(fault.Forbidden, "FORBIDDEN", "Доступно только admin.")
	}
	current, err := w.repo.Read(ctx, subjectID)
	if err != nil {
		return coursegraph.Snapshot{}, err
	}
	if current.Status == "pending" || current.Status == "processing" {
		return coursegraph.Snapshot{}, fault.New(fault.Conflict, "GRAPH_BUILD_IN_PROGRESS", "Граф курса уже собирается.")
	}
	items, err := w.materials.List(ctx, subjectID)
	if err != nil {
		return coursegraph.Snapshot{}, err
	}
	missing := []string{}
	allProcessed := len(items) > 0
	for _, item := range items {
		if item.Status != material.StatusParsed || item.Cleaning == nil || item.Cleaning.Status != "ready" {
			allProcessed = false
		}
		if item.Week == nil || *item.Week <= 0 || item.Kind == nil {
			missing = append(missing, item.ID)
		}
	}
	if allProcessed && len(missing) > 0 {
		f := fault.New(fault.Invalid, "MATERIAL_METADATA_INCOMPLETE", "Классифицируйте неделю и вид у каждого материала перед сборкой графа.")
		for _, id := range missing {
			f.Details = append(f.Details, fault.Detail{Path: "materials", Code: "MATERIAL_METADATA_INCOMPLETE", Message: id})
		}
		return coursegraph.Snapshot{}, f
	}
	queued, err := w.repo.Request(ctx, subjectID, time.Now())
	if err != nil {
		return coursegraph.Snapshot{}, err
	}
	snapshot, err := w.snapshot(ctx, subjectID)
	switch {
	case err != nil:
		return coursegraph.Snapshot{}, err
	case queued:
		return snapshot, nil
	case snapshot.Status == "pending" || snapshot.Status == "processing":
		return coursegraph.Snapshot{}, fault.New(fault.Conflict, "GRAPH_BUILD_IN_PROGRESS", "Граф курса уже собирается.")
	default:
		return coursegraph.Snapshot{}, fault.New(fault.Invalid, "GRAPH_INPUTS_NOT_READY",
			"Граф можно собрать, когда все материалы курса обработаны.")
	}
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
	if owner, ok := w.repo.(interface {
		Renew(context.Context, coursegraph.Job, time.Time) (bool, error)
		Release(context.Context, coursegraph.Job, time.Time) error
	}); ok {
		err = runWithLease(ctx, job, owner.Renew, owner.Release, func(taskCtx context.Context) error { return w.process(taskCtx, job) })
	} else {
		err = w.process(ctx, job)
	}
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, errGraphLeaseLost) {
			return true, nil
		}
		failureCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		if failure := w.repo.Fail(failureCtx, job, "Не удалось построить граф; предыдущий граф сохранён.", time.Now()); failure != nil {
			return true, fmt.Errorf("%v; record failure: %w", err, failure)
		}
	}
	return true, err
}

var errGraphLeaseLost = errors.New("course graph lease lost")

func runWithLease(ctx context.Context, job coursegraph.Job,
	renew func(context.Context, coursegraph.Job, time.Time) (bool, error),
	release func(context.Context, coursegraph.Job, time.Time) error,
	process func(context.Context) error) error {
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
				releaseCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				_ = release(releaseCtx, job, time.Now())
				stop()
				return ctx.Err()
			}
			return err
		case <-ctx.Done():
			cancel()
			<-done
			releaseCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			_ = release(releaseCtx, job, time.Now())
			stop()
			return ctx.Err()
		case <-ticker.C:
			heartbeatCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			owned, err := renew(heartbeatCtx, job, time.Now())
			stop()
			if err != nil || !owned {
				cancel()
				<-done
				if err != nil {
					return err
				}
				return errGraphLeaseLost
			}
		}
	}
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
		var kind *string
		if m.Kind != nil {
			value := string(*m.Kind)
			kind = &value
		}
		revision := sha256.Sum256([]byte(m.S3RawKey))
		input := coursegraph.Material{ID: m.ID, Markdown: string(source), Week: m.Week, Kind: kind,
			Filename: m.Filename, KindLabel: m.KindLabel, MaterialRevision: hex.EncodeToString(revision[:])}
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
	if json.Unmarshal(graph, &shape) != nil || shape.Version != "course-graph-v1" || !shape.Quality.Acyclic || len(shape.Topics) == 0 || shape.Prerequisites == nil {
		return fmt.Errorf("invalid graph result")
	}
	// The questions are voiced in the background before the graph is
	// published, so the first students already hear them from the cache.
	if w.voices != nil {
		questions := TopicQuestions(graph)
		log.Printf("course graph %s: queued %d of %d questions for synthesis", job.SubjectID, w.voices.Prepare(questions), len(questions))
	}
	accepted, err := w.repo.Finish(ctx, job, graph, time.Now())
	if err != nil || accepted {
		return err
	}
	// The materials changed during the build: queue it again so the next
	// claim takes the new set, without spending an attempt.
	if owner, ok := w.repo.(interface {
		Release(context.Context, coursegraph.Job, time.Time) error
	}); ok {
		return owner.Release(context.WithoutCancel(ctx), job, time.Now())
	}
	return nil
}
