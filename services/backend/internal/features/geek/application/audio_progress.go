package application

import (
	"sync"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audiopreparation"
)

// ExamAudioProgress is the in-process view of published course speech keys.
type ExamAudioProgress struct {
	mu    sync.RWMutex
	voice string
	sets  map[string]examAudioSet
	now   func() time.Time
}

type examAudioSet struct {
	version    string
	keys       map[string]audiopreparation.Status
	observedAt time.Time
}

func NewExamAudioProgress(voice string) *ExamAudioProgress {
	return &ExamAudioProgress{voice: voice, sets: map[string]examAudioSet{}, now: time.Now}
}

func (p *ExamAudioProgress) TrackPublished(subjectID, graphVersion string, texts []string, key func(string) string) {
	keys := make(map[string]audiopreparation.Status, len(texts))
	for _, text := range texts {
		if normalized := key(text); normalized != "" {
			keys[normalized] = audiopreparation.StatusUnchecked
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sets[subjectID] = examAudioSet{version: graphVersion + "|" + p.voice, keys: keys, observedAt: p.now()}
}

func (p *ExamAudioProgress) Transition(key string, state audiopreparation.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, set := range p.sets {
		if _, ok := set.keys[key]; !ok {
			continue
		}
		set.keys[key] = state
		set.observedAt = p.now()
		p.sets[id] = set
	}
}

func (p *ExamAudioProgress) Snapshot(subjectID string) (audiopreparation.ExamSnapshot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	set, ok := p.sets[subjectID]
	if !ok {
		return audiopreparation.ExamSnapshot{}, false
	}
	out := audiopreparation.ExamSnapshot{Version: set.version, Total: len(set.keys), ObservedAt: set.observedAt}
	for _, state := range set.keys {
		switch state {
		case audiopreparation.StatusUnchecked:
			out.Unchecked++
		case audiopreparation.StatusQueued:
			out.Queued++
		case audiopreparation.StatusProcessing:
			out.Processing++
		case audiopreparation.StatusRetryWait:
			out.RetryWait++
		case audiopreparation.StatusReady:
			out.Ready++
		case audiopreparation.StatusFailed:
			out.Failed++
		case audiopreparation.StatusUnavailable:
			out.Unavailable++
		default:
			out.Unavailable++
		}
	}
	return out, true
}
