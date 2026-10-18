package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	geekapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	geekgraph "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/graph"
)

// buildExaminer fails the first fails questions, then writes questions by
// topic title and rephrases by the question.
type buildExaminer struct {
	fails atomic.Int32
	asked atomic.Int32
}

func (e *buildExaminer) Question(_ context.Context, r geekapp.QuestionRequest) (string, error) {
	e.asked.Add(1)
	if e.fails.Add(-1) >= 0 {
		return "", errors.New("gateway down")
	}
	if !r.Ahead {
		return "", errors.New("not prepared ahead")
	}
	if r.Kind == exam.KindRephrase {
		return "Проще: " + r.Dialogue[0].Text, nil
	}
	return "Вопрос про «" + r.Node.Title + "»?", nil
}

func (e *buildExaminer) Evaluate(context.Context, geekapp.EvaluationRequest) (geekapp.Assessment, error) {
	return geekapp.Assessment{}, errors.New("not used")
}

func (e *buildExaminer) Feedback(context.Context, geekapp.FeedbackRequest) (string, error) {
	return "", errors.New("not used")
}

const build = `{"version":"course-graph-v1","quality":{"acyclic":true},"hierarchy":[{"id":"s"}],
"topics":[
 {"id":"a","title":"Поиск","text":"Текст поиска","question":"Как работает поиск?","provides":["поиск"],"requires":[],"sources":[{"material_id":"m","block_id":1,"start":0,"end":5,"text":"Текст"}],"extra":{"kept":true}},
 {"id":"b","title":"Эвристика","text":"Текст эвристики","question":"","provides":["эвристика"],"requires":["поиск"]}
],
"prerequisites":[{"source":"a","target":"b"}]}`

func TestBuiltQuestionsArePreparedIntoTheTopics(t *testing.T) {
	examiner := &buildExaminer{}
	// The first round loses two calls; the next round prepares the rest.
	examiner.fails.Store(2)
	out, err := builtQuestions{examiner: examiner, logger: zap.NewNop(), pause: time.Millisecond}.Prepare(context.Background(), json.RawMessage(build))
	if err != nil {
		t.Fatal(err)
	}
	g, ok, err := geekgraph.Decode(out)
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if len(geekapp.Unprepared(g)) != 0 {
		t.Fatalf("unprepared: %+v", g.Nodes)
	}
	a, _ := g.Node("a")
	b, _ := g.Node("b")
	if a.MainQuestion() != "Как работает поиск?" || a.Rephrase != "Проще: Как работает поиск?" ||
		b.MainQuestion() != "Вопрос про «Эвристика»?" || b.Rephrase != "Проще: Вопрос про «Эвристика»?" {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	if len(b.Prerequisites) != 1 || b.Prerequisites[0] != "a" || len(a.Sources) != 1 {
		t.Fatalf("graph changed: a=%+v b=%+v", a, b)
	}
	// Everything course-graph wrote is kept.
	for _, field := range []string{`"hierarchy":[{"id":"s"}]`, `"extra":{"kept":true}`, `"quality":{"acyclic":true}`, `"version":"course-graph-v1"`} {
		if !strings.Contains(string(out), field) {
			t.Fatalf("lost %s in %s", field, out)
		}
	}
}

func TestBuiltQuestionsFailWhenATopicStaysUnprepared(t *testing.T) {
	examiner := &buildExaminer{}
	examiner.fails.Store(1000)
	_, err := builtQuestions{examiner: examiner, logger: zap.NewNop(), pause: time.Millisecond}.Prepare(context.Background(), json.RawMessage(build))
	if err == nil || !strings.Contains(err.Error(), "2 topics without prepared questions") {
		t.Fatalf("err = %v", err)
	}
	// Each of the three rounds asks for each topic once and stops at its failure.
	if examiner.asked.Load() != 2*builtQuestionRounds {
		t.Fatalf("asked %d times", examiner.asked.Load())
	}
}
