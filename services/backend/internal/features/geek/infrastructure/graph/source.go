// Package graph supplies course graphs to Geek: from the database once the
// graph is stored there, otherwise from a fixture file.
package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
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
