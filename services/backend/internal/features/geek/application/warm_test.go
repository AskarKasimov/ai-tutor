package application

import (
	"context"
	"sync"
	"testing"
	"time"

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
	warmer := NewWarmer(NewSpeechCache(voice, store, "steps=20"))
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

func TestMainQuestionsAreThePreparedFirstQuestions(t *testing.T) {
	g := exam.Graph{Nodes: []exam.Node{
		{ID: "a", Questions: []string{" Почему? ", "Ещё"}},
		{ID: "b"},
		{ID: "c", Questions: []string{"  "}},
	}}
	if got := MainQuestions(g); len(got) != 1 || got[0] != "Почему?" {
		t.Fatalf("questions = %q", got)
	}
}
