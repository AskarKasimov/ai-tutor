// Package testkit runs the production voice handler/provider with an isolated journal.
// It is used by backend integration tests; model HTTP servers remain test-owned.
package testkit

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/execution"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/gateway"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/journal"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/provider"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/scheduler"
	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/testutil"
)

type Fixture struct {
	*gateway.Server
	mu             sync.Mutex
	sttURL, ttsURL string
	profile        string
	opts           provider.Options
}

func New(t testing.TB, sttURL, ttsURL, profile string) *Fixture {
	t.Helper()
	f := &Fixture{sttURL: sttURL, ttsURL: ttsURL, profile: profile, opts: provider.Options{Seed: 17, CFGValue: 2, InferenceTimesteps: 50}}
	blobs, err := journal.OpenBlobs(t.TempDir(), 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(blobs.Close)
	stt, _ := scheduler.New([]scheduler.Instance{{ID: "stt", Slots: 1}}, 32)
	tts, _ := scheduler.New([]scheduler.Instance{{ID: "tts", Slots: 1, Profiles: []string{profile}}}, 32)
	// Test HTTP servers stop processing when their contexts are cancelled.
	engine := execution.New(&testutil.Memory{}, blobs, map[string]*scheduler.Queue{"stt": stt, "tts": tts}, time.Hour, func(string) bool { return true }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := engine.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Wait)
	f.Server = gateway.New(engine, fixtureProvider{f}, map[string]string{profile: "fixture-v1"}, 10*time.Second)
	return f
}
func (f *Fixture) SetSTTURL(url string) { f.mu.Lock(); f.sttURL = url; f.mu.Unlock() }
func (f *Fixture) SetTTSURL(url string) { f.mu.Lock(); f.ttsURL = url; f.mu.Unlock() }
func (f *Fixture) SetModes(reference, prompt bool) {
	f.mu.Lock()
	f.opts.UseReference = reference
	f.opts.UsePrompt = prompt
	f.mu.Unlock()
}
func (f *Fixture) httpProvider() *provider.HTTP {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &provider.HTTP{STT: map[string]provider.STT{"stt": {URL: f.sttURL, Timeout: 10 * time.Second}}, TTS: map[string]provider.TTS{"tts": {URL: f.ttsURL, Timeout: 10 * time.Second, Profiles: map[string]provider.Options{f.profile: f.opts}}}}
}

type fixtureProvider struct{ f *Fixture }

func (p fixtureProvider) Transcribe(ctx context.Context, instance string, data []byte, media string) (string, error) {
	return p.f.httpProvider().Transcribe(ctx, instance, data, media)
}
func (p fixtureProvider) Synthesize(ctx context.Context, instance, profile, text string) ([]byte, error) {
	return p.f.httpProvider().Synthesize(ctx, instance, profile, text)
}
