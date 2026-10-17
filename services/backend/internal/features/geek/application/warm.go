package application

import (
	"context"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// warmQueue bounds the questions waiting for synthesis; a full queue drops
// the rest, which are then voiced when a student first hears them.
const warmQueue = 4096

// Warmer voices prepared questions in the background, one at a time, before
// any student asks for them. The speech service works through requests one by
// one and the warmer has no priority: while it runs, live turns wait behind
// its current request.
type Warmer struct {
	speech *SpeechCache
	queue  chan string
}

func NewWarmer(speech *SpeechCache) *Warmer {
	return &Warmer{speech: speech, queue: make(chan string, warmQueue)}
}

// Prepare queues texts for synthesis without waiting; it reports how many
// were queued. A text already cached costs one cache read.
func (w *Warmer) Prepare(texts []string) int {
	queued := 0
	for _, text := range texts {
		if text = strings.TrimSpace(text); text == "" {
			continue
		}
		select {
		case w.queue <- text:
			queued++
		default:
			return queued
		}
	}
	return queued
}

// Run synthesizes queued texts until ctx ends. A failure skips the text: the
// voice adapter logs it, and the question is voiced on demand later.
func (w *Warmer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case text := <-w.queue:
			_, _ = w.speech.Part(ctx, text)
		}
	}
}

// MainQuestions are the prepared main questions of a graph: the text Geek
// asks first on each topic, voiced as the question part of its turn.
func MainQuestions(g exam.Graph) []string {
	out := []string{}
	for _, n := range g.Nodes {
		if len(n.Questions) > 0 && strings.TrimSpace(n.Questions[0]) != "" {
			out = append(out, strings.TrimSpace(n.Questions[0]))
		}
	}
	return out
}
