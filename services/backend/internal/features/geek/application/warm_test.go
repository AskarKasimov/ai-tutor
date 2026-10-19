package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
)

// countingVoice is safe for the warmer goroutine and reports each synthesis.
type countingVoice struct {
	mu     sync.Mutex
	calls  []string
	called chan struct{}
}

func (v *countingVoice) Synthesize(_ context.Context, text string) ([]byte, error) {
	v.mu.Lock()
	v.calls = append(v.calls, text)
	v.mu.Unlock()
	v.called <- struct{}{}
	return wav(10), nil
}

type syncAudio struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (a *syncAudio) Get(_ context.Context, key string) ([]byte, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, ok := a.data[key]
	return data, ok, nil
}

func (a *syncAudio) size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.data)
}

func (a *syncAudio) Put(_ context.Context, key string, data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.data[key] = data
	return nil
}

func TestWarmerVoicesPreparedQuestionsOnceBeforeStudentsAsk(t *testing.T) {
	voice := &countingVoice{called: make(chan struct{}, 8)}
	store := &syncAudio{data: map[string][]byte{}}
	warmer := NewWarmer(NewSpeechCache(voice, store, "steps=20"), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = warmer.Run(ctx); close(done) }()

	if n := warmer.Prepare([]string{" Почему A* оптимален? ", "", "Зачем эвристика?"}); n != 2 {
		t.Fatalf("queued %d", n)
	}
	<-voice.called
	<-voice.called
	// The signal comes from the synthesis; the WAV is cached right after it.
	for deadline := time.Now().Add(5 * time.Second); store.size() < 2; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the warmed questions were not cached")
		}
	}
	// A question already in the cache is not synthesized again.
	warmer.Prepare([]string{"Почему A* оптимален?"})
	speech := NewSpeechCache(voice, store, "steps=20")
	if _, err := speech.Part(context.Background(), "Зачем эвристика?"); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if len(voice.calls) != 2 || voice.calls[0] != "Почему A* оптимален?" || voice.calls[1] != "Зачем эвристика?" {
		t.Fatalf("synthesized %q", voice.calls)
	}
}

func TestPreparedQuestionsAreTheMainQuestionsAndTheirRephrases(t *testing.T) {
	g := exam.Graph{Nodes: []exam.Node{
		{ID: "a", Questions: []string{" Почему? ", "Ещё"}, Rephrase: " Отчего? "},
		{ID: "b"},
		// A rephrase of no prepared question is generated at runtime.
		{ID: "c", Questions: []string{"  "}, Rephrase: "Сирота"},
		{ID: "d", Questions: []string{"Зачем?"}},
	}}
	if got := PreparedQuestions(g); !slices.Equal(got, []string{"Почему?", "Отчего?", "Зачем?"}) {
		t.Fatalf("questions = %q", got)
	}
}

// flakyAudio fails reads until healthy is set, like a bucket that is not
// created yet.
type flakyAudio struct {
	syncAudio
	healthy atomic.Bool
	fails   atomic.Int32
}

func (a *flakyAudio) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if !a.healthy.Load() {
		a.fails.Add(1)
		return nil, false, errors.New("NoSuchBucket")
	}
	return a.syncAudio.Get(ctx, key)
}

func runWarmer(t *testing.T, w *Warmer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

func TestWarmerRetriesStoreFailuresUntilVoiced(t *testing.T) {
	voice := &countingVoice{called: make(chan struct{}, 8)}
	// The bucket is not created yet when the warmer starts.
	store := &flakyAudio{syncAudio: syncAudio{data: map[string][]byte{}}}
	core, logs := observer.New(zap.WarnLevel)
	warmer := NewWarmer(NewSpeechCache(voice, store, "steps=20"), zap.New(core))
	warmer.retryMin, warmer.retryMax = time.Millisecond, 2*time.Millisecond
	runWarmer(t, warmer)

	warmer.Prepare([]string{"Первый?", "Второй?"})
	// Every failure is logged and retried, none lost.
	waitFor(t, "failures were not retried", func() bool { return store.fails.Load() >= 4 })
	if logs.FilterMessage("prepared question not voiced; retrying").Len() == 0 {
		t.Fatal("the failure was not logged")
	}
	if warmer.Pending() != 2 {
		t.Fatalf("pending = %d", warmer.Pending())
	}
	store.healthy.Store(true)
	waitFor(t, "the questions were not voiced after the store recovered", func() bool { return store.size() == 2 })
	waitFor(t, "the queue was not emptied", func() bool { return warmer.Pending() == 0 })
}

// failingVoice fails the first fail syntheses.
type failingVoice struct {
	countingVoice
	fail atomic.Int32
}

func (v *failingVoice) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if v.fail.Add(-1) >= 0 {
		return nil, errors.New("speech service is down")
	}
	return v.countingVoice.Synthesize(ctx, text)
}

func TestWarmerRetriesAFailedSynthesisBehindTheOtherTexts(t *testing.T) {
	voice := &failingVoice{countingVoice: countingVoice{called: make(chan struct{}, 8)}}
	voice.fail.Store(1)
	store := &syncAudio{data: map[string][]byte{}}
	warmer := NewWarmer(NewSpeechCache(voice, store, "steps=20"), nil)
	warmer.retryMin, warmer.retryMax = time.Millisecond, time.Millisecond
	runWarmer(t, warmer)

	warmer.Prepare([]string{"Первый?", "Второй?"})
	<-voice.called
	<-voice.called
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if !slices.Equal(voice.calls, []string{"Второй?", "Первый?"}) {
		t.Fatalf("synthesized %q", voice.calls)
	}
}

func TestWarmerQueuesAWaitingTextOnceAndHasNoQueueLimit(t *testing.T) {
	store := &syncAudio{data: map[string][]byte{}}
	// Not running: everything stays queued.
	warmer := NewWarmer(NewSpeechCache(&countingVoice{called: make(chan struct{}, 1)}, store, "steps=20"), nil)
	texts := make([]string, 5000)
	for i := range texts {
		texts[i] = fmt.Sprintf("Вопрос %d?", i)
	}
	if n := warmer.Prepare(texts); n != len(texts) {
		t.Fatalf("queued %d", n)
	}
	if n := warmer.Prepare([]string{"Вопрос 1?", " Вопрос 2? ", "Вопрос 1?"}); n != 0 || warmer.Pending() != len(texts) {
		t.Fatalf("queued %d again, pending %d", n, warmer.Pending())
	}
}

// gatedVoice holds every synthesis until release is closed.
type gatedVoice struct {
	countingVoice
	release chan struct{}
}

func (v *gatedVoice) Synthesize(ctx context.Context, text string) ([]byte, error) {
	data, err := v.countingVoice.Synthesize(ctx, text)
	<-v.release
	return data, err
}

func TestConcurrentMissesOfOneQuestionSynthesizeOnce(t *testing.T) {
	voice := &gatedVoice{countingVoice: countingVoice{called: make(chan struct{}, 8)}, release: make(chan struct{})}
	speech := NewSpeechCache(voice, &syncAudio{data: map[string][]byte{}}, "steps=20")
	// The warmer starts the synthesis, then a student leaves while waiting
	// and another one asks for the same question.
	warmer := make(chan error, 1)
	go func() { _, err := speech.Part(context.Background(), "Зачем эвристика?"); warmer <- err }()
	<-voice.called
	leaving, leave := context.WithCancel(context.Background())
	left := make(chan error, 1)
	go func() { _, err := speech.Part(leaving, "Зачем эвристика?"); left <- err }()
	staying := make(chan error, 1)
	go func() { _, err := speech.Part(context.Background(), "Зачем эвристика?"); staying <- err }()
	leave()
	if err := <-left; err == nil {
		t.Fatal("the student who left still waited for the synthesis")
	}
	close(voice.release)
	if err := <-warmer; err != nil {
		t.Fatal(err)
	}
	if err := <-staying; err != nil {
		t.Fatal(err)
	}
	voice.mu.Lock()
	defer voice.mu.Unlock()
	if len(voice.calls) != 1 {
		t.Fatalf("the same question was synthesized %d times", len(voice.calls))
	}
}
