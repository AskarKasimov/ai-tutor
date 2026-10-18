package application

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// Pauses between rounds of preparing a graph that still has unprepared nodes.
const (
	prepareRetryMin = 5 * time.Second
	prepareRetryMax = 5 * time.Minute
)

// storeAttempts bounds the reads of prepared texts that fail, storePause
// times the attempt apart; after them the graph is prepared through the model.
const (
	storeAttempts = 3
	storePause    = time.Second
)

// PreparedStore keeps the texts prepared for a graph version, so a restart
// reuses them instead of asking the model for new wording, which would also
// be voiced again.
type PreparedStore interface {
	Get(ctx context.Context, key string) (data []byte, ok bool, err error)
	Put(ctx context.Context, key string, data []byte) error
}

// PreparedGraph serves the one graph of its source, the same for every course,
// such as the fixture, only once every node has its main question and
// rephrase prepared (decision of 18.10.2026): until then a new diagnostic on
// it does not start, and diagnostics already started go on by their own copy
// of the graph. The prepared graph keeps the version of the source, so
// preparing again after a restart does not cancel them; a new version of the
// source, such as an edited fixture file, is prepared again. The texts are
// kept in store by version, so a restart asks the model only for what is
// missing.
type PreparedGraph struct {
	source     GraphSource
	examiner   Examiner
	store      PreparedStore
	voices     interface{ Prepare([]string) int }
	logger     *zap.Logger
	parallel   int
	retryMin   time.Duration
	retryMax   time.Duration
	storePause time.Duration

	mu    sync.RWMutex
	graph *exam.Graph
	// stale wakes Run when the source has a version other than the graph's.
	stale chan struct{}
}

// NewPreparedGraph prepares the graph of source; store may be nil, then
// every start prepares it again. voices, if not nil, gets the prepared texts
// to synthesize. logger may be nil.
func NewPreparedGraph(source GraphSource, examiner Examiner, store PreparedStore, voices interface{ Prepare([]string) int }, logger *zap.Logger) *PreparedGraph {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PreparedGraph{source: source, examiner: examiner, store: store, voices: voices, logger: logger, parallel: 4,
		retryMin: prepareRetryMin, retryMax: prepareRetryMax, storePause: storePause, stale: make(chan struct{}, 1)}
}

// Current is the prepared graph; ok is false while the source has no graph,
// while it is being prepared and while the source has a newer version than
// the prepared one. A source that cannot be read is an error, as without the
// preparation.
func (p *PreparedGraph) Current(ctx context.Context, _ string) (exam.Graph, bool, error) {
	source, ok, err := p.source.Current(ctx, "")
	if err != nil || !ok {
		return exam.Graph{}, false, err
	}
	g := p.prepared()
	if g == nil || g.Version != source.Version {
		select {
		case p.stale <- struct{}{}:
		default:
		}
		return exam.Graph{}, false, nil
	}
	return *g, true, nil
}

// Run prepares the graph and prepares it again whenever Current finds a newer
// version of the source, until ctx ends.
func (p *PreparedGraph) Run(ctx context.Context) error {
	for {
		p.prepare(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-p.stale:
		}
	}
}

// prepare brings the served graph to the version of the source: it takes the
// texts kept for that version, asks the model for the rest and retries the
// unprepared nodes after a growing pause. The source is read again every
// round, so a new version replaces one that cannot be prepared. The main
// questions already in the source are voiced at once, the rest once
// prepared. It returns when the graph is served, when the source has none, or
// when ctx ends.
func (p *PreparedGraph) prepare(ctx context.Context) {
	pause := p.retryMin
	var g exam.Graph
	for loaded := false; ctx.Err() == nil; {
		source, ok, err := p.source.Current(ctx, "")
		switch {
		case err != nil:
			p.logger.Warn("graph is not loaded for preparation; retrying", zap.Error(err), zap.Duration("retry_in", pause))
		case !ok:
			p.set(nil)
			return
		default:
			if served := p.prepared(); served != nil && served.Version == source.Version {
				return
			}
			if !loaded || g.Version != source.Version {
				g, loaded = p.restore(ctx, source), true
				p.voice(g)
			}
			started := time.Now()
			prepared, count, failures := PrepareQuestions(ctx, p.examiner, g, p.parallel)
			g = prepared
			if count > 0 {
				p.keep(ctx, g)
				p.voice(g)
			}
			left := Unprepared(g)
			if len(left) == 0 {
				p.set(&g)
				p.logger.Info("graph questions prepared", zap.String("version", g.Version), zap.Int("prepared", count), zap.Int("nodes", len(g.Nodes)), zap.Duration("duration", time.Since(started)))
				return
			}
			if ctx.Err() != nil {
				return
			}
			p.logger.Warn("graph questions are not all prepared; retrying", zap.Error(failures), zap.Strings("nodes", left), zap.Duration("retry_in", pause))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
		pause = min(2*pause, p.retryMax)
	}
}

// preparedTexts is what the store keeps for a version: the main question and
// rephrase of each node by its ID.
type preparedTexts struct {
	Version string `json:"version"`
	Nodes   map[string]struct {
		Question string `json:"question"`
		Rephrase string `json:"rephrase"`
	} `json:"nodes"`
}

func storeKey(version string) string {
	return "geek/prepared/" + strings.NewReplacer(":", "-", "/", "-").Replace(version) + ".json"
}

// restore fills g from the texts kept for its version. A text is taken only
// where the node has no main question of its own, and a rephrase only with
// the main question it rephrases. A store that keeps failing is skipped: the
// model prepares the graph instead.
func (p *PreparedGraph) restore(ctx context.Context, g exam.Graph) exam.Graph {
	if p.store == nil {
		return g
	}
	var data []byte
	for attempt := 1; ; attempt++ {
		got, ok, err := p.store.Get(ctx, storeKey(g.Version))
		if err == nil {
			if !ok {
				return g
			}
			data = got
			break
		}
		if attempt == storeAttempts || ctx.Err() != nil {
			p.logger.Warn("prepared graph texts are not read; preparing them", zap.Error(err), zap.String("version", g.Version))
			return g
		}
		select {
		case <-ctx.Done():
			return g
		case <-time.After(time.Duration(attempt) * p.storePause):
		}
	}
	var kept preparedTexts
	if err := json.Unmarshal(data, &kept); err != nil || kept.Version != g.Version {
		p.logger.Warn("prepared graph texts are not usable; preparing them", zap.Error(err), zap.String("version", g.Version))
		return g
	}
	out := g
	out.Nodes = make([]exam.Node, len(g.Nodes))
	copy(out.Nodes, g.Nodes)
	for i := range out.Nodes {
		n := &out.Nodes[i]
		t, ok := kept.Nodes[n.ID]
		if !ok {
			continue
		}
		question := strings.TrimSpace(t.Question)
		if n.MainQuestion() == "" && question != "" {
			n.Questions = append([]string{question}, n.Questions[min(1, len(n.Questions)):]...)
			n.Rephrase = ""
		}
		if question != "" && n.PreparedRephrase() == "" && n.MainQuestion() == question {
			n.Rephrase = strings.TrimSpace(t.Rephrase)
		}
	}
	return out
}

// keep stores the prepared texts of g; a failure is logged, and the next
// start prepares them again.
func (p *PreparedGraph) keep(ctx context.Context, g exam.Graph) {
	if p.store == nil {
		return
	}
	kept := preparedTexts{Version: g.Version, Nodes: map[string]struct {
		Question string `json:"question"`
		Rephrase string `json:"rephrase"`
	}{}}
	for _, n := range g.Nodes {
		if n.MainQuestion() == "" {
			continue
		}
		t := kept.Nodes[n.ID]
		t.Question, t.Rephrase = n.MainQuestion(), n.PreparedRephrase()
		kept.Nodes[n.ID] = t
	}
	data, err := json.Marshal(kept)
	if err == nil {
		err = p.store.Put(ctx, storeKey(g.Version), data)
	}
	if err != nil {
		p.logger.Warn("prepared graph texts are not kept", zap.Error(err), zap.String("version", g.Version))
	}
}

func (p *PreparedGraph) prepared() *exam.Graph {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.graph
}

func (p *PreparedGraph) set(g *exam.Graph) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.graph = g
}

func (p *PreparedGraph) voice(g exam.Graph) {
	if p.voices != nil {
		p.voices.Prepare(PreparedQuestions(g))
	}
}
