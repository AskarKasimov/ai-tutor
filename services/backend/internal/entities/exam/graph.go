// Package exam holds the oral exam domain: the course graph, topic grading and
// the traversal rules of the diagnostic.
package exam

import (
	"errors"
	"fmt"
	"strings"
)

// Node is a topic of the course graph. Prerequisites reference other nodes;
// outcomes and questions may be empty until they are prepared.
type Node struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Prerequisites []string `json:"prerequisites"`
	Outcomes      []string `json:"outcomes"`
	Questions     []string `json:"questions"`
}

// Graph is the course graph: nodes in study order and acyclic prerequisite
// edges. Version changes whenever the graph is rebuilt.
type Graph struct {
	Version string `json:"version"`
	Nodes   []Node `json:"nodes"`
}

var ErrInvalidGraph = errors.New("invalid course graph")

// Validate checks that IDs are unique and non-empty, prerequisites exist and
// the prerequisite graph has no cycles.
func (g Graph) Validate() error {
	if len(g.Nodes) == 0 {
		return fmt.Errorf("%w: no nodes", ErrInvalidGraph)
	}
	index := make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		if strings.TrimSpace(n.ID) == "" || strings.TrimSpace(n.Title) == "" {
			return fmt.Errorf("%w: node %d needs an id and a title", ErrInvalidGraph, i)
		}
		if _, dup := index[n.ID]; dup {
			return fmt.Errorf("%w: duplicate node %q", ErrInvalidGraph, n.ID)
		}
		index[n.ID] = i
	}
	for _, n := range g.Nodes {
		for _, p := range n.Prerequisites {
			if _, ok := index[p]; !ok {
				return fmt.Errorf("%w: node %q needs unknown prerequisite %q", ErrInvalidGraph, n.ID, p)
			}
		}
	}
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(g.Nodes))
	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("%w: prerequisite cycle through %q", ErrInvalidGraph, id)
		case done:
			return nil
		}
		state[id] = visiting
		for _, p := range g.Nodes[index[id]].Prerequisites {
			if err := visit(p); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for _, n := range g.Nodes {
		if err := visit(n.ID); err != nil {
			return err
		}
	}
	return nil
}

// Node returns the node with the given ID.
func (g Graph) Node(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// TopLevel returns, in study order, the nodes that are not a prerequisite of
// any other node. The exam starts from them and the course grade averages them.
func (g Graph) TopLevel() []string {
	required := map[string]bool{}
	for _, n := range g.Nodes {
		for _, p := range n.Prerequisites {
			required[p] = true
		}
	}
	top := []string{}
	for _, n := range g.Nodes {
		if !required[n.ID] {
			top = append(top, n.ID)
		}
	}
	return top
}

// Closure returns the transitive prerequisites of a node, without the node,
// in depth-first order.
func (g Graph) Closure(id string) []string {
	seen := map[string]bool{id: true}
	out := []string{}
	var walk func(string)
	walk = func(id string) {
		n, _ := g.Node(id)
		for _, p := range n.Prerequisites {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
				walk(p)
			}
		}
	}
	walk(id)
	return out
}
