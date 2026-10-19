package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// PrepareQuestions fills the main question and its rephrase of every node
// that lacks them, so the exam asks them at once instead of waiting for the
// model. Existing texts are kept; a main question generated here gets a new
// rephrase. A node that could not be prepared keeps what it had, and the exam
// generates the rest at runtime as before; the failures are returned joined.
// prepared counts the nodes completed by this call. parallel bounds the
// concurrent nodes.
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
	fail := func(id string, err error) {
		mu.Lock()
		defer mu.Unlock()
		failures = append(failures, fmt.Errorf("%s: %w", id, err))
	}
	slots := make(chan struct{}, max(1, parallel))
	for i := range out.Nodes {
		node := out.Nodes[i]
		if node.Prepared() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				fail(node.ID, ctx.Err())
				return
			}
			defer func() { <-slots }()
			main := node.MainQuestion()
			if main == "" {
				question, err := prepare(ctx, examiner, QuestionRequest{Kind: exam.KindQuestion, Node: node, Ahead: true})
				if err != nil {
					fail(node.ID, err)
					return
				}
				main = question
				questions := []string{main}
				if len(node.Questions) > 1 {
					questions = append(questions, node.Questions[1:]...)
				}
				node.Questions = questions
				node.Rephrase = ""
				// The main question is kept even if its rephrase fails.
				mu.Lock()
				out.Nodes[i].Questions = questions
				out.Nodes[i].Rephrase = ""
				mu.Unlock()
			}
			// The rephrase prompt reads the question it rephrases from the
			// dialogue, as at runtime, but without a student's answer.
			asked := []exam.Turn{{Line: 1, Speaker: exam.SpeakerGeek, Kind: exam.KindQuestion, NodeID: &node.ID, Text: main}}
			rephrase, err := prepare(ctx, examiner, QuestionRequest{Kind: exam.KindRephrase, Node: node, Dialogue: asked, Ahead: true})
			if err != nil {
				fail(node.ID, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			out.Nodes[i].Rephrase = rephrase
			prepared++
		}()
	}
	wg.Wait()
	return out, prepared, errors.Join(failures...)
}

func prepare(ctx context.Context, examiner Examiner, r QuestionRequest) (string, error) {
	text, err := examiner.Question(ctx, r)
	text = strings.TrimSpace(text)
	if err == nil && text == "" {
		err = errors.New("empty " + string(r.Kind))
	}
	return text, err
}

// Unprepared lists the nodes without a prepared main question or rephrase.
func Unprepared(g exam.Graph) []string {
	out := []string{}
	for _, n := range g.Nodes {
		if !n.Prepared() {
			out = append(out, n.ID)
		}
	}
	return out
}
