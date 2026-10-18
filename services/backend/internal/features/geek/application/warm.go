package application

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// Retry pauses of a text that failed: they grow from warmRetryMin to
// warmRetryMax and reset after a success. The cap keeps a text that cannot be
// cached (its synthesis succeeds, the store refuses it) from occupying the
// speech service that live turns wait behind.
const (
	warmRetryMin = time.Second
	warmRetryMax = 5 * time.Minute
)

// Warmer voices prepared questions in the background, one at a time, before
// any student asks for them. The speech service works through requests one by
// one and the warmer has no priority: while it runs, live turns wait behind
// its current request. A failed text goes back to the end of the queue after
// a pause and is retried until it is voiced, so a speech service or storage
// that is not up yet, such as a bucket still being created at startup, delays
// the warming instead of losing it.
type Warmer struct {
	speech             *SpeechCache
	logger             *zap.Logger
	retryMin, retryMax time.Duration

	mu sync.Mutex
	// queue holds texts in order; pending is the same set, so a text queued
	// again before it is voiced is not voiced twice.
	queue   []string
	pending map[string]bool
	wake    chan struct{}
}

// NewWarmer builds the warmer; logger may be nil.
func NewWarmer(speech *SpeechCache, logger *zap.Logger) *Warmer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Warmer{speech: speech, logger: logger, retryMin: warmRetryMin, retryMax: warmRetryMax,
		pending: map[string]bool{}, wake: make(chan struct{}, 1)}
}

// Prepare queues texts for synthesis without waiting; it reports how many
// were queued. A text already waiting is not queued again; a text already
// cached costs one cache read.
func (w *Warmer) Prepare(texts []string) int {
	w.mu.Lock()
	queued := 0
	for _, text := range texts {
		if text = strings.TrimSpace(text); text == "" || w.pending[text] {
			continue
		}
		w.pending[text] = true
		w.queue = append(w.queue, text)
		queued++
	}
	w.mu.Unlock()
	if queued > 0 {
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
	return queued
}

// Pending is the number of texts waiting to be voiced.
func (w *Warmer) Pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.queue)
}

// Run synthesizes queued texts until ctx ends. A failure is logged and the
// text is retried after a growing pause, behind the other waiting texts.
func (w *Warmer) Run(ctx context.Context) error {
	pause := w.retryMin
	for {
		text, ok := w.next()
		if !ok {
			select {
			case <-ctx.Done():
				return nil
			case <-w.wake:
				continue
			}
		}
		_, err := w.speech.WarmPart(ctx, text)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			w.done(text)
			pause = w.retryMin
			continue
		}
		w.retry(text)
		w.logger.Warn("prepared question not voiced; retrying", zap.Error(err), zap.Int("text_chars", len([]rune(text))), zap.Duration("retry_in", pause), zap.Int("pending", w.Pending()))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pause):
		}
		pause = min(2*pause, w.retryMax)
	}
}

func (w *Warmer) next() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) == 0 {
		return "", false
	}
	return w.queue[0], true
}

func (w *Warmer) done(text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.queue = w.queue[1:]
	delete(w.pending, text)
}

// retry moves the failed head text to the end of the queue.
func (w *Warmer) retry(text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.queue = append(w.queue[1:], text)
}

// PreparedQuestions are the prepared texts of a graph that Geek asks as is:
// the main question of each topic and its rephrase.
func PreparedQuestions(g exam.Graph) []string {
	out := []string{}
	for _, n := range g.Nodes {
		if q := n.MainQuestion(); q != "" {
			out = append(out, q)
		}
		if r := n.PreparedRephrase(); r != "" {
			out = append(out, r)
		}
	}
	return out
}
