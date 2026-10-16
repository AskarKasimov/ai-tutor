package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

type preparing struct {
	failOn   string
	running  atomic.Int32
	peak     atomic.Int32
	requests atomic.Int32
}

func (p *preparing) Question(_ context.Context, r QuestionRequest) (string, error) {
	p.requests.Add(1)
	now := p.running.Add(1)
	defer p.running.Add(-1)
	for {
		peak := p.peak.Load()
		if now <= peak || p.peak.CompareAndSwap(peak, now) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	if r.Kind != exam.KindQuestion {
		return "", errors.New("not a main question")
	}
	if r.Node.ID == p.failOn {
		return "", errors.New("gateway down")
	}
	return "Вопрос про «" + r.Node.Title + "»?", nil
}

func (p *preparing) Evaluate(context.Context, EvaluationRequest) (exam.Evaluation, error) {
	return exam.Evaluation{}, errors.New("not used")
}

func TestPrepareQuestionsFillsOnlyMissingMainQuestions(t *testing.T) {
	g := loadGraph(t)
	// Only a-star keeps its question; the others start empty.
	for i := range g.Nodes {
		if g.Nodes[i].ID != "a-star" {
			g.Nodes[i].Questions = nil
		}
	}
	p := &preparing{failOn: "mcts"}
	out, prepared, err := PrepareQuestions(context.Background(), p, g, 2)
	if err == nil {
		t.Fatal("the failed node is reported")
	}
	// The fixture has 10 nodes; a-star already has a question and mcts fails.
	if prepared != 8 || p.requests.Load() != 9 || p.peak.Load() > 2 {
		t.Fatalf("prepared=%d requests=%d peak=%d", prepared, p.requests.Load(), p.peak.Load())
	}
	for _, n := range out.Nodes {
		switch n.ID {
		case "a-star":
			if n.Questions[0] != g.Nodes[4].Questions[0] {
				t.Fatalf("existing question replaced: %v", n.Questions)
			}
		case "mcts":
			if len(n.Questions) != 0 {
				t.Fatalf("failed node got %v", n.Questions)
			}
		default:
			if len(n.Questions) != 1 || n.Questions[0] != "Вопрос про «"+n.Title+"»?" {
				t.Fatalf("%s questions = %v", n.ID, n.Questions)
			}
		}
	}
	for _, n := range g.Nodes {
		if n.ID != "a-star" && len(n.Questions) != 0 {
			t.Fatal("the input graph was changed")
		}
	}
}
