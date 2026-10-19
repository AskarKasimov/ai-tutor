package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	for i := range out.Nodes {
		out.Nodes[i].Variants = slices.Clone(out.Nodes[i].Variants)
	}
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
			if len(node.Variants) > 0 {
				// Every task of a map topic is asked as given; each gets its
				// own rephrase, and the node keeps those of the first.
				for j, v := range node.Variants {
					if node.WithVariant(v).Prepared() {
						continue
					}
					rephrase, err := rephraseOf(ctx, examiner, node.WithVariant(v), v.Question)
					if err != nil {
						fail(node.ID, err)
						return
					}
					mu.Lock()
					out.Nodes[i].Variants[j].Rephrase = rephrase
					if j == 0 {
						out.Nodes[i].Rephrase = rephrase
					}
					mu.Unlock()
				}
				mu.Lock()
				prepared++
				mu.Unlock()
				return
			}
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
			rephrase, err := rephraseOf(ctx, examiner, node, main)
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

// rephraseOf prepares the rephrase of the question main of node. The prompt
// reads the question it rephrases from the dialogue, as at runtime, but
// without a student's answer.
func rephraseOf(ctx context.Context, examiner Examiner, node exam.Node, main string) (string, error) {
	asked := []exam.Turn{{Line: 1, Speaker: exam.SpeakerGeek, Kind: exam.KindQuestion, NodeID: &node.ID, Text: main}}
	return prepare(ctx, examiner, QuestionRequest{Kind: exam.KindRephrase, Node: node, Dialogue: asked, Ahead: true})
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
