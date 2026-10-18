package application

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

type preparing struct {
	failOn         string
	failRephraseOn string
	running        atomic.Int32
	peak           atomic.Int32
	requests       atomic.Int32
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
	if !r.Ahead {
		return "", errors.New("not prepared ahead")
	}
	switch r.Kind {
	case exam.KindQuestion:
		if r.Node.ID == p.failOn {
			return "", errors.New("gateway down")
		}
		return "Вопрос про «" + r.Node.Title + "»?", nil
	case exam.KindRephrase:
		// The rephrase sees only the question it rephrases, no answer.
		if len(r.Dialogue) != 1 || r.Dialogue[0].Speaker != exam.SpeakerGeek {
			return "", errors.New("rephrase without its question")
		}
		if r.Node.ID == p.failRephraseOn {
			return "", errors.New("gateway down")
		}
		return "Проще: " + r.Dialogue[0].Text, nil
	}
	return "", errors.New("unexpected kind " + string(r.Kind))
}

func (p *preparing) Evaluate(context.Context, EvaluationRequest) (Assessment, error) {
	return Assessment{}, errors.New("not used")
}

func (p *preparing) Explain(context.Context, EvaluationRequest) (string, error) {
	return "", errors.New("not used")
}

func TestPrepareQuestionsFillsMissingMainQuestionsAndRephrases(t *testing.T) {
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
	// The fixture has 10 nodes: eight get both texts, a-star only its
	// rephrase, and mcts fails on its main question.
	if prepared != 9 || p.requests.Load() != 18 || p.peak.Load() > 2 {
		t.Fatalf("prepared=%d requests=%d peak=%d", prepared, p.requests.Load(), p.peak.Load())
	}
	if left := Unprepared(out); !slices.Equal(left, []string{"mcts"}) {
		t.Fatalf("unprepared = %v", left)
	}
	for _, n := range out.Nodes {
		switch n.ID {
		case "a-star":
			if n.Questions[0] != g.Nodes[4].Questions[0] || n.Rephrase != "Проще: "+n.Questions[0] {
				t.Fatalf("a-star = %q, %q", n.Questions, n.Rephrase)
			}
		case "mcts":
			if len(n.Questions) != 0 || n.Rephrase != "" {
				t.Fatalf("failed node got %q, %q", n.Questions, n.Rephrase)
			}
		default:
			if len(n.Questions) != 1 || n.Questions[0] != "Вопрос про «"+n.Title+"»?" || n.Rephrase != "Проще: "+n.Questions[0] {
				t.Fatalf("%s = %q, %q", n.ID, n.Questions, n.Rephrase)
			}
		}
	}
	for _, n := range g.Nodes {
		if n.ID != "a-star" && len(n.Questions) != 0 || n.Rephrase != "" {
			t.Fatal("the input graph was changed")
		}
	}
}

func TestPrepareQuestionsKeepsRemainingQuestionsWhenMainIsEmpty(t *testing.T) {
	for _, first := range []string{"", " \t\n"} {
		t.Run(first, func(t *testing.T) {
			questions := []string{first, "Существующий второй вопрос?", "Существующий третий вопрос?"}
			before := slices.Clone(questions)
			g := exam.Graph{Nodes: []exam.Node{{ID: "topic", Title: "Тема", Content: "Текст темы", Questions: questions}}}
			if err := g.Validate(); err != nil {
				t.Fatal(err)
			}
			out, count, err := PrepareQuestions(context.Background(), &preparing{}, g, 1)
			if err != nil || count != 1 {
				t.Fatalf("prepared=%d err=%v", count, err)
			}
			want := []string{"Вопрос про «Тема»?", before[1], before[2]}
			if !slices.Equal(out.Nodes[0].Questions, want) || out.Nodes[0].Rephrase != "Проще: "+want[0] {
				t.Fatalf("questions=%q rephrase=%q, want %q", out.Nodes[0].Questions, out.Nodes[0].Rephrase, want)
			}
			if !slices.Equal(g.Nodes[0].Questions, before) {
				t.Fatalf("input changed: %q", g.Nodes[0].Questions)
			}
		})
	}
}

func TestPrepareQuestionsKeepsTheMainQuestionWhenItsRephraseFails(t *testing.T) {
	g := exam.Graph{Nodes: []exam.Node{{ID: "topic", Title: "Тема", Content: "Текст темы"}}}
	out, count, err := PrepareQuestions(context.Background(), &preparing{failRephraseOn: "topic"}, g, 1)
	if err == nil || count != 0 {
		t.Fatalf("prepared=%d err=%v", count, err)
	}
	if n := out.Nodes[0]; n.MainQuestion() != "Вопрос про «Тема»?" || n.Rephrase != "" || n.Prepared() {
		t.Fatalf("node = %+v", n)
	}
	// The next round asks only for the missing rephrase.
	p := &preparing{}
	out, count, err = PrepareQuestions(context.Background(), p, out, 1)
	if err != nil || count != 1 || p.requests.Load() != 1 || out.Nodes[0].Rephrase != "Проще: Вопрос про «Тема»?" {
		t.Fatalf("prepared=%d requests=%d err=%v node=%+v", count, p.requests.Load(), err, out.Nodes[0])
	}
}

func TestPrepareQuestionsLeavesPreparedNodesAndDropsARephraseOfNoQuestion(t *testing.T) {
	g := exam.Graph{Nodes: []exam.Node{
		{ID: "ready", Title: "Готово", Questions: []string{"Есть?"}, Rephrase: "Есть ли?"},
		// A rephrase without a main question rephrases nothing Geek asks.
		{ID: "orphan", Title: "Сирота", Rephrase: "Старая переформулировка"},
	}}
	p := &preparing{}
	out, count, err := PrepareQuestions(context.Background(), p, g, 1)
	if err != nil || count != 1 || p.requests.Load() != 2 {
		t.Fatalf("prepared=%d requests=%d err=%v", count, p.requests.Load(), err)
	}
	if out.Nodes[0].Rephrase != "Есть ли?" || out.Nodes[1].Rephrase != "Проще: Вопрос про «Сирота»?" {
		t.Fatalf("nodes = %+v", out.Nodes)
	}
}
