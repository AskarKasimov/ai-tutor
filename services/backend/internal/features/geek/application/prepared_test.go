package application

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// versionedSource serves a graph whose version a test can change.
type versionedSource struct {
	mu    sync.Mutex
	graph exam.Graph
}

func (s *versionedSource) Current(context.Context, string) (exam.Graph, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.graph, true, nil
}

func (s *versionedSource) bump(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.graph.Version = version
}

// flakyExaminer fails every question until healthy is set.
type flakyExaminer struct {
	preparing
	healthy atomic.Bool
}

func (e *flakyExaminer) Question(ctx context.Context, r QuestionRequest) (string, error) {
	if !e.healthy.Load() {
		return "", errors.New("gateway down")
	}
	return e.preparing.Question(ctx, r)
}

type recordingVoices struct {
	mu    sync.Mutex
	texts []string
}

func (v *recordingVoices) Prepare(texts []string) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.texts = append(v.texts, texts...)
	return len(texts)
}

func (v *recordingVoices) has(text string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.Contains(v.texts, text)
}

func TestPreparedGraphIsServedOnlyOnceEveryNodeIsPrepared(t *testing.T) {
	source := &versionedSource{graph: exam.Graph{Version: "fixture:1", Nodes: []exam.Node{
		{ID: "a", Title: "А", Questions: []string{"Готовый вопрос?"}},
		{ID: "b", Title: "Б"},
	}}}
	examiner := &flakyExaminer{}
	voices := &recordingVoices{}
	p := NewPreparedGraph(source, examiner, nil, voices, nil)
	p.retryMin, p.retryMax, p.storePause = time.Millisecond, 2*time.Millisecond, time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = p.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	// The gateway is down: the graph is not served, and the main question
	// already in the source is voiced meanwhile.
	waitFor(t, "the ready main question was not voiced", func() bool { return voices.has("Готовый вопрос?") })
	time.Sleep(20 * time.Millisecond)
	if _, ok, err := p.Current(ctx, "subject"); ok || err != nil {
		t.Fatalf("served an unprepared graph: ok=%v err=%v", ok, err)
	}

	examiner.healthy.Store(true)
	var g exam.Graph
	waitFor(t, "the graph was not served after the gateway recovered", func() bool {
		var ok bool
		g, ok, _ = p.Current(ctx, "subject")
		return ok
	})
	if g.Version != "fixture:1" || len(Unprepared(g)) != 0 {
		t.Fatalf("graph = %+v", g)
	}
	for _, text := range PreparedQuestions(g) {
		if !voices.has(text) {
			t.Fatalf("%q is not voiced ahead", text)
		}
	}

	// A new version of the source is not served until it is prepared again.
	before := examiner.requests.Load()
	source.bump("fixture:2")
	if _, ok, _ := p.Current(ctx, "subject"); ok {
		t.Fatal("served the previous version")
	}
	waitFor(t, "the new version was not prepared", func() bool {
		g, ok, _ := p.Current(ctx, "subject")
		return ok && g.Version == "fixture:2"
	})
	// The version is prepared once, though every Current while it was being
	// prepared signalled it: a's rephrase, b's question and rephrase.
	time.Sleep(50 * time.Millisecond)
	if n := examiner.requests.Load() - before; n != 3 {
		t.Fatalf("the new version took %d model calls", n)
	}
}

func TestPreparedGraphWithoutSourceServesNothing(t *testing.T) {
	p := NewPreparedGraph(&graphs{}, &preparing{}, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = p.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	time.Sleep(10 * time.Millisecond)
	if _, ok, err := p.Current(ctx, "subject"); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func runPrepared(t *testing.T, p *PreparedGraph) {
	t.Helper()
	p.retryMin, p.retryMax, p.storePause = time.Millisecond, 2*time.Millisecond, time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = p.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
}

func servedGraph(t *testing.T, p *PreparedGraph, version string) exam.Graph {
	t.Helper()
	var g exam.Graph
	waitFor(t, "graph "+version+" was not served", func() bool {
		var ok bool
		g, ok, _ = p.Current(context.Background(), "subject")
		return ok && g.Version == version
	})
	return g
}

// memoryPrepared is a PreparedStore whose reads fail while failing is set
// and whose writes fail while putFailing is set.
type memoryPrepared struct {
	mu         sync.Mutex
	data       map[string][]byte
	failing    atomic.Bool
	putFailing atomic.Bool
	reads      atomic.Int32
}

func (m *memoryPrepared) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.reads.Add(1)
	if m.failing.Load() {
		return nil, false, errors.New("S3 is down")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.data[key]
	return data, ok, nil
}

func (m *memoryPrepared) Put(_ context.Context, key string, data []byte) error {
	if m.putFailing.Load() {
		return errors.New("S3 is down")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func fixtureSource() *versionedSource {
	return &versionedSource{graph: exam.Graph{Version: "fixture:1", Nodes: []exam.Node{
		{ID: "a", Title: "А", Questions: []string{"Готовый вопрос?"}},
		{ID: "b", Title: "Б"},
	}}}
}

func TestPreparedGraphTextsSurviveARestart(t *testing.T) {
	store := &memoryPrepared{data: map[string][]byte{}}
	first := &preparing{}
	p := NewPreparedGraph(fixtureSource(), first, store, nil, nil)
	runPrepared(t, p)
	want := servedGraph(t, p, "fixture:1")
	if first.requests.Load() != 3 {
		t.Fatalf("first start took %d model calls", first.requests.Load())
	}

	// After a restart the same version is served with the same texts,
	// without the model, so their audio is already cached.
	second := &preparing{}
	voices := &recordingVoices{}
	restarted := NewPreparedGraph(fixtureSource(), second, store, voices, nil)
	runPrepared(t, restarted)
	got := servedGraph(t, restarted, "fixture:1")
	if second.requests.Load() != 0 {
		t.Fatalf("the restart took %d model calls", second.requests.Load())
	}
	if !slices.Equal(PreparedQuestions(got), PreparedQuestions(want)) {
		t.Fatalf("texts changed: %q, want %q", PreparedQuestions(got), PreparedQuestions(want))
	}
	for _, text := range PreparedQuestions(want) {
		if !voices.has(text) {
			t.Fatalf("%q is not voiced ahead", text)
		}
	}
}

func (m *memoryPrepared) has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok
}

func TestPreparedGraphStoresTheTextsOnceTheStoreRecovers(t *testing.T) {
	store := &memoryPrepared{data: map[string][]byte{}}
	store.putFailing.Store(true)
	first := &preparing{}
	p := NewPreparedGraph(fixtureSource(), first, store, nil, nil)
	runPrepared(t, p)
	want := servedGraph(t, p, "fixture:1")

	// The texts already served are stored once S3 is back, without the model.
	store.putFailing.Store(false)
	waitFor(t, "texts were not stored after the store recovered", func() bool { return store.has(storeKey("fixture:1")) })
	if first.requests.Load() != 3 {
		t.Fatalf("storing took %d model calls", first.requests.Load())
	}

	second := &preparing{}
	restarted := NewPreparedGraph(fixtureSource(), second, store, nil, nil)
	runPrepared(t, restarted)
	got := servedGraph(t, restarted, "fixture:1")
	if second.requests.Load() != 0 || !slices.Equal(PreparedQuestions(got), PreparedQuestions(want)) {
		t.Fatalf("the restart took %d model calls, texts %q, want %q", second.requests.Load(), PreparedQuestions(got), PreparedQuestions(want))
	}
}

func TestPreparedGraphTakesOnlyTextsOfItsQuestions(t *testing.T) {
	store := &memoryPrepared{data: map[string][]byte{
		storeKey("fixture:1"): []byte(`{"version":"fixture:1","nodes":{
			"a":{"question":"Старый вопрос?","rephrase":"Старая переформулировка?"},
			"b":{"question":"Вопрос Б?","rephrase":"Проще: Вопрос Б?"}}}`),
		// Another version's texts are never read for this one.
		storeKey("fixture:0"): []byte(`{"version":"fixture:0","nodes":{}}`),
	}}
	examiner := &preparing{}
	p := NewPreparedGraph(fixtureSource(), examiner, store, nil, nil)
	runPrepared(t, p)
	g := servedGraph(t, p, "fixture:1")
	a, _ := g.Node("a")
	b, _ := g.Node("b")
	// a keeps its own question, so the kept rephrase of another one is not
	// taken: only a's rephrase is asked for.
	if a.MainQuestion() != "Готовый вопрос?" || a.Rephrase != "Проще: Готовый вопрос?" ||
		b.MainQuestion() != "Вопрос Б?" || b.Rephrase != "Проще: Вопрос Б?" || examiner.requests.Load() != 1 {
		t.Fatalf("a=%+v b=%+v requests=%d", a, b, examiner.requests.Load())
	}
}

func TestPreparedGraphPreparesThroughTheModelWhenTheStoreFails(t *testing.T) {
	store := &memoryPrepared{data: map[string][]byte{}}
	store.failing.Store(true)
	p := NewPreparedGraph(fixtureSource(), &preparing{}, store, nil, nil)
	runPrepared(t, p)
	servedGraph(t, p, "fixture:1")
	if store.reads.Load() != storeAttempts {
		t.Fatalf("store read %d times", store.reads.Load())
	}
}

func TestPreparedGraphReportsAnUnreadableSource(t *testing.T) {
	p := NewPreparedGraph(brokenSource{}, &preparing{}, nil, nil, nil)
	runPrepared(t, p)
	// A broken fixture is an error, as before, not a graph still preparing.
	if _, ok, err := p.Current(context.Background(), "subject"); ok || err == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

type brokenSource struct{}

func (brokenSource) Current(context.Context, string) (exam.Graph, bool, error) {
	return exam.Graph{}, false, errors.New("decode graph fixture: unexpected end of JSON input")
}

func TestPreparedGraphTakesANewVersionWhileTheOldOneCannotBePrepared(t *testing.T) {
	source := fixtureSource()
	// b of version 1 always fails; version 2 fixes the topic.
	examiner := &preparing{failOn: "b"}
	p := NewPreparedGraph(source, examiner, nil, nil, nil)
	runPrepared(t, p)
	waitFor(t, "version 1 was not tried", func() bool { return examiner.requests.Load() >= 4 })
	source.mu.Lock()
	source.graph = exam.Graph{Version: "fixture:2", Nodes: []exam.Node{{ID: "a", Title: "А", Questions: []string{"Готовый вопрос?"}}, {ID: "c", Title: "В"}}}
	source.mu.Unlock()
	servedGraph(t, p, "fixture:2")
}
