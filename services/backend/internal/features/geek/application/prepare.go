package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// PrepareQuestions fills the main question of every node that has none, so
// the exam asks it at once instead of waiting for the model. Existing
// questions are kept. A node whose question could not be generated stays
// empty, and the exam generates its question at runtime as before; the
// failures are returned joined. parallel bounds the concurrent model calls.
func PrepareQuestions(ctx context.Context, examiner Examiner, g exam.Graph, parallel int) (exam.Graph, int, error) {
	out := g
	out.Nodes = make([]exam.Node, len(g.Nodes))
	copy(out.Nodes, g.Nodes)
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		failures []error
		prepared int
	)
	slots := make(chan struct{}, max(1, parallel))
	for i := range out.Nodes {
		node := out.Nodes[i]
		if len(node.Questions) > 0 && strings.TrimSpace(node.Questions[0]) != "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				failures = append(failures, fmt.Errorf("%s: %w", node.ID, ctx.Err()))
				mu.Unlock()
				return
			}
			defer func() { <-slots }()
			question, err := examiner.Question(ctx, QuestionRequest{Kind: exam.KindQuestion, Node: node})
			question = strings.TrimSpace(question)
			if err == nil && question == "" {
				err = errors.New("empty question")
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", node.ID, err))
				return
			}
			questions := []string{question}
			if len(node.Questions) > 1 {
				questions = append(questions, node.Questions[1:]...)
			}
			out.Nodes[i].Questions = questions
			prepared++
		}()
	}
	wg.Wait()
	return out, prepared, errors.Join(failures...)
}
