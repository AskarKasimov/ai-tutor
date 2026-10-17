// Package graph supplies course graphs to Geek: from the database once the
// graph is stored there, otherwise from a fixture file.
package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	db "github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/postgres/sqlcgen"
)

// Chain returns the graph of the first source that has one.
type Chain []application.GraphSource

func (c Chain) Current(ctx context.Context, subjectID string) (exam.Graph, bool, error) {
	for _, source := range c {
		if source == nil {
			continue
		}
		g, ok, err := source.Current(ctx, subjectID)
		if err != nil || ok {
			return g, ok, err
		}
	}
	return exam.Graph{}, false, nil
}

// Fixture serves one graph file for every course. The version is the hash of
// the file, so editing it counts as a rebuild.
type Fixture struct{ Path string }

func (f Fixture) Current(context.Context, string) (exam.Graph, bool, error) {
	if f.Path == "" {
		return exam.Graph{}, false, nil
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return exam.Graph{}, false, fmt.Errorf("read graph fixture: %w", err)
	}
	var g exam.Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return exam.Graph{}, false, fmt.Errorf("decode graph fixture: %w", err)
	}
	if err := g.Validate(); err != nil {
		return exam.Graph{}, false, err
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		n.Prerequisites, n.Outcomes, n.Questions = orEmpty(n.Prerequisites), orEmpty(n.Outcomes), orEmpty(n.Questions)
	}
	sum := sha256.Sum256(data)
	g.Version = "fixture:" + hex.EncodeToString(sum[:8])
	return g, true, nil
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// Stored reads the course graph from PostgreSQL.
type Stored struct {
	pool *pgxpool.Pool
}

func NewStored(pool *pgxpool.Pool) Stored {
	return Stored{pool: pool}
}

// Current is the published graph of the course. A database failure is an
// error, not a missing graph, so the chain never hides it behind the fixture.
// The version is the hash of the published graph: any rebuild that changes it
// is a new version, which cancels active diagnostics. raw.Version is the
// format of the course-graph result, not the build.
func (s Stored) Current(ctx context.Context, subjectID string) (exam.Graph, bool, error) {
	if s.pool == nil {
		return exam.Graph{}, false, nil
	}
	data, err := db.New(s.pool).ReadPublishedCourseGraph(ctx, subjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return exam.Graph{}, false, nil
	}
	if err != nil {
		return exam.Graph{}, false, fmt.Errorf("read stored graph: %w", err)
	}
	if len(data) == 0 || string(data) == "null" {
		return exam.Graph{}, false, nil
	}
	var raw struct {
		Version string      `json:"version"`
		Nodes   []exam.Node `json:"nodes"`
		Topics  []struct {
			ID       string   `json:"id"`
			Title    string   `json:"title"`
			Text     string   `json:"text"`
			Content  string   `json:"content"`
			Question string   `json:"question"`
			Provides []string `json:"provides"`
			Requires []string `json:"requires"`
		} `json:"topics"`
		Prerequisites []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"prerequisites"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return exam.Graph{}, false, fmt.Errorf("decode stored graph: %w", err)
	}
	var g exam.Graph
	if len(raw.Nodes) > 0 {
		g.Nodes = raw.Nodes
	} else if len(raw.Topics) > 0 {
		prereqsByTarget := make(map[string][]string)
		for _, edge := range raw.Prerequisites {
			prereqsByTarget[edge.Target] = append(prereqsByTarget[edge.Target], edge.Source)
		}
		g.Nodes = make([]exam.Node, len(raw.Topics))
		for i, t := range raw.Topics {
			content := t.Content
			if content == "" {
				content = t.Text
			}
			var questions []string
			if strings.TrimSpace(t.Question) != "" {
				questions = []string{strings.TrimSpace(t.Question)}
			}
			g.Nodes[i] = exam.Node{
				ID:            t.ID,
				Title:         t.Title,
				Content:       content,
				Prerequisites: prereqsByTarget[t.ID],
				Outcomes:      t.Provides,
				Questions:     questions,
			}
		}
	} else {
		return exam.Graph{}, false, nil
	}

	if err := g.Validate(); err != nil {
		return exam.Graph{}, false, err
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		n.Prerequisites, n.Outcomes, n.Questions = orEmpty(n.Prerequisites), orEmpty(n.Outcomes), orEmpty(n.Questions)
	}
	sum := sha256.Sum256(data)
	g.Version = "pg:" + hex.EncodeToString(sum[:8])
	return g, true, nil
}
