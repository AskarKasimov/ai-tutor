package application

import (
	"bytes"
	"context"
	"errors"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/metrics"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type streamVoice struct {
	calls     atomic.Int32
	started   chan struct{}
	release   chan struct{}
	cancelled chan struct{}
}

func (v *streamVoice) Synthesize(ctx context.Context, _ string) ([]byte, error) {
	v.calls.Add(1)
	close(v.started)
	select {
	case <-ctx.Done():
		close(v.cancelled)
		return nil, ctx.Err()
	case <-v.release:
		return wav(10), nil
	}
}
func (v *streamVoice) SynthesizeStream(ctx context.Context, _ string, emit func(audio.StreamEvent) error) error {
	v.calls.Add(1)
	if err := emit(audio.StartEvent(24000)); err != nil {
		return err
	}
	if err := emit(audio.StreamEvent{Type: "audio", PCM: []byte{1, 0, 2, 0}}); err != nil {
		return err
	}
	close(v.started)
	select {
	case <-ctx.Done():
		close(v.cancelled)
		return ctx.Err()
	case <-v.release:
		return emit(audio.StreamEvent{Type: "end", Chunks: 1, Samples: 2})
	}
}

type noWriteAudio struct{ syncAudio }

func (a *noWriteAudio) Put(context.Context, string, []byte) error {
	return errors.New("S3 unavailable")
}
func TestStreamSharesProducerWithWAVAndSurvivesCacheWriteFailure(t *testing.T) {
	v := &streamVoice{started: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{})}
	store := &noWriteAudio{syncAudio{data: map[string][]byte{}}}
	c := NewSpeechCache(v, store, "profile")
	live := make(chan struct{})
	streamDone := make(chan error, 1)
	go func() {
		streamDone <- c.Stream(context.Background(), "key", "question", func(e audio.StreamEvent) error {
			if e.Type == "audio" {
				close(live)
			}
			return nil
		})
	}()
	select {
	case <-live:
	case <-time.After(time.Second):
		t.Fatal("not live")
	}
	<-v.started
	wavDone := make(chan error, 1)
	go func() {
		data, err := c.Get(context.Background(), "key", "question")
		if err == nil && !audio.ValidWAV(data) {
			err = errors.New("invalid WAV")
		}
		wavDone <- err
	}()
	waitFor(t, "follower not joined", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.flights[c.cacheKey("key")].users == 2 })
	close(v.release)
	if err := <-streamDone; err != nil {
		t.Fatal(err)
	}
	if err := <-wavDone; err != nil {
		t.Fatal(err)
	}
	if v.calls.Load() != 1 {
		t.Fatal("duplicate synthesis")
	}
}
func TestLastListenerCancellationReachesProvider(t *testing.T) {
	v := &streamVoice{started: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{})}
	c := NewSpeechCache(v, &syncAudio{data: map[string][]byte{}}, "profile")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Get(ctx, "key", "question"); done <- err }()
	<-v.started
	cancel()
	if <-done == nil {
		t.Fatal("cancellation ignored")
	}
	select {
	case <-v.cancelled:
	case <-time.After(time.Second):
		t.Fatal("producer leaked")
	}
}
func TestTurnStreamChecksOwnerAndPartsBeforeGateAndCache(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s, _, _ := f.service.Start(ctx, "student", "subject", "start")
	turn := s.Turns[0]
	emit := func(audio.StreamEvent) error { return nil }
	if err := f.service.TurnAudioStream(ctx, "other", s.ID, turn.ID, AudioWhole, emit); code(err) != "SESSION_NOT_FOUND" {
		t.Fatalf("ownership %v", err)
	}
	if err := f.service.TurnAudioStream(ctx, "student", s.ID, turn.ID, AudioReply, emit); code(err) != "TURN_PART_NOT_FOUND" {
		t.Fatalf("part %v", err)
	}
	if err := f.service.TurnAudioStream(ctx, "student", s.ID, turn.ID, AudioWhole, emit); code(err) != "STREAMING_DISABLED" {
		t.Fatalf("gate %v", err)
	}
	if len(f.voice.calls) != 0 {
		t.Fatal("synthesized before authorization")
	}
	f.service.cfg.StreamingEnabled = true
	var frames []audio.StreamEvent
	if err := f.service.TurnAudioStream(ctx, "student", s.ID, turn.ID, AudioWhole, func(e audio.StreamEvent) error { frames = append(frames, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(frames) < 3 || frames[0].Type != "start" || frames[len(frames)-1].Type != "end" {
		t.Fatalf("frames %v", frames)
	}
}

type brokenStreamVoice struct{}

func (brokenStreamVoice) Synthesize(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected WAV fallback")
}
func (brokenStreamVoice) SynthesizeStream(_ context.Context, _ string, emit func(audio.StreamEvent) error) error {
	_ = emit(audio.StartEvent(24000))
	_ = emit(audio.StreamEvent{Type: "audio", Seq: 0, PCM: []byte{1, 0}})
	return errors.New("provider disconnect")
}
func TestStreamFailureNeverPublishesPartialCache(t *testing.T) {
	store := &syncAudio{data: map[string][]byte{}}
	c := NewSpeechCache(brokenStreamVoice{}, store, "profile")
	ended := false
	err := c.StreamPart(context.Background(), "question", func(e audio.StreamEvent) error { ended = ended || e.Type == "end"; return nil })
	if err == nil || ended || store.size() != 0 {
		t.Fatalf("partial published ended=%v err=%v", ended, err)
	}
}
func TestWAVProducerKeepsRetriesAndStreamFollowerWaits(t *testing.T) {
	v := &streamVoice{started: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{})}
	c := NewSpeechCache(v, &syncAudio{data: map[string][]byte{}}, "profile")
	done := make(chan error, 1)
	go func() { _, err := c.Part(context.Background(), "question"); done <- err }()
	<-v.started
	streamDone := make(chan error, 1)
	var got []audio.StreamEvent
	go func() {
		streamDone <- c.StreamPart(context.Background(), "question", func(e audio.StreamEvent) error { got = append(got, e); return nil })
	}()
	waitFor(t, "stream follower not joined", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.flights[c.cacheKey(c.partKey("question"))].users == 2
	})
	close(v.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-streamDone; err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || v.calls.Load() != 1 {
		t.Fatalf("frames=%d calls=%d", len(got), v.calls.Load())
	}
}

type fastStreamVoice struct {
	calls    atomic.Int32
	produced atomic.Int32
	finished chan struct{}
}

func (v *fastStreamVoice) Synthesize(context.Context, string) ([]byte, error) { return wav(10), nil }
func (v *fastStreamVoice) SynthesizeStream(ctx context.Context, _ string, emit func(audio.StreamEvent) error) error {
	v.calls.Add(1)
	defer func() { v.finished <- struct{}{} }()
	if err := emit(audio.StartEvent(24000)); err != nil {
		return err
	}
	for i := 0; i < 40; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		v.produced.Add(1)
		if err := emit(audio.StreamEvent{Type: "audio", Seq: i, PCM: make([]byte, audio.StreamBlockBytes)}); err != nil {
			return err
		}
	}
	return emit(audio.StreamEvent{Type: "end", Chunks: 40, Samples: 40 * audio.StreamBlockBytes / 2})
}
func TestSlowStreamReaderHasEightBlockQueueAndCancelsProducer(t *testing.T) {
	v := &fastStreamVoice{finished: make(chan struct{}, 2)}
	store := &syncAudio{data: map[string][]byte{}}
	c := NewSpeechCache(v, store, "profile")
	blocked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.StreamPart(context.Background(), "question", func(e audio.StreamEvent) error {
			if e.Type == "audio" {
				select {
				case <-blocked:
				default:
					close(blocked)
				}
				<-release
			}
			return nil
		})
	}()
	<-blocked
	select {
	case <-v.finished:
	case <-time.After(7 * time.Second):
		t.Fatal("slow reader did not cancel provider")
	}
	// One consumed audio, eight queued blocks, and one attempted blocked send.
	if n := v.produced.Load(); n > 10 {
		t.Fatalf("unbounded queue: %d audio blocks", n)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("slow stream succeeded")
	}
	if store.size() != 0 {
		t.Fatal("partial cached")
	}
	// The same service remains available for another request after drain timeout.
	if err := c.StreamPart(context.Background(), "next", func(audio.StreamEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type timeoutWriteAudio struct{ syncAudio }

func (a *timeoutWriteAudio) Put(ctx context.Context, _ string, _ []byte) error {
	<-ctx.Done()
	return ctx.Err()
}

type completedStreamVoice struct{}

func (completedStreamVoice) Synthesize(context.Context, string) ([]byte, error) { return wav(10), nil }
func (completedStreamVoice) SynthesizeStream(_ context.Context, _ string, emit func(audio.StreamEvent) error) error {
	return audio.StreamWAV(wav(10), emit)
}
func TestCacheWriteTimeoutDoesNotInvalidateCompletedStream(t *testing.T) {
	for range 16 {
		c := NewSpeechCache(completedStreamVoice{}, &timeoutWriteAudio{syncAudio{data: map[string][]byte{}}}, "profile", 50*time.Millisecond)
		ended := false
		if err := c.StreamPart(context.Background(), "Вопрос?", func(e audio.StreamEvent) error { ended = ended || e.Type == "end"; return nil }); err != nil || !ended {
			t.Fatalf("complete audio lost after cache timeout: end=%v err=%v", ended, err)
		}
	}
}

func TestStreamMetricsRecordCacheAndCompletion(t *testing.T) {
	m := metrics.NewVoice()
	c := NewSpeechCache(completedStreamVoice{}, &syncAudio{data: map[string][]byte{}}, "profile").WithMetrics(m)
	for range 2 {
		if err := c.StreamPart(context.Background(), "Вопрос?", func(audio.StreamEvent) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	_ = m.WritePrometheus(&b)
	for _, want := range []string{`backend_tts_cache_total{result="miss"} 1`, `backend_tts_cache_total{result="written"} 1`, `backend_tts_cache_total{result="hit"} 1`, `backend_tts_stream_requests_total{source="leader",outcome="success"} 2`, `backend_tts_stream_ttfa_seconds_count{source="leader"} 2`} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %s", want)
		}
	}
}
