package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestServeWithBackgroundStopsWorkerAfterSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	stopped := make(chan struct{})
	var runs atomic.Int32
	worker := func(ctx context.Context) error {
		runs.Add(1)
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- serveWithBackground(ctx, &http.Server{Addr: "127.0.0.1:0"}, worker) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("background task did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server lifecycle did not stop")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("background task was not joined")
	}
	if runs.Load() != 1 {
		t.Fatalf("worker runs=%d", runs.Load())
	}
}

func TestServeWithBackgroundStopsWorkerWhenListenFails(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	worker := func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil
	}
	err := serveWithBackground(context.Background(), &http.Server{Addr: ":invalid"}, worker)
	if err == nil {
		t.Fatal("invalid listener unexpectedly succeeded")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background task did not start")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("background task was not joined after ListenAndServe error")
	}
}
