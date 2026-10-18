package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

// SplitSpeech cuts text into parts of at most limit runes for synthesis,
// preferring sentence ends, then spaces; a word longer than limit is cut.
func SplitSpeech(text string, limit int) []string {
	text = strings.TrimSpace(text)
	parts := []string{}
	for utf8.RuneCountInString(text) > limit {
		runes := []rune(text)
		cut := -1
		for i := limit - 1; i > limit/2; i-- {
			if strings.ContainsRune(".!?…", runes[i]) {
				cut = i + 1
				break
			}
		}
		if cut < 0 {
			for i := limit; i > limit/2; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
		}
		if cut < 0 {
			cut = limit
		}
		parts = append(parts, strings.TrimSpace(string(runes[:cut])))
		text = strings.TrimSpace(string(runes[cut:]))
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}

// SpeechCache shares one bounded producer among WAV, streaming and warmer consumers.
type SpeechCache struct {
	logger  *zap.Logger
	voice   Synthesizer
	audio   AudioStore
	name    string
	timeout time.Duration
	mu      sync.Mutex
	flights map[string]*speechFlight
}
type speechFlight struct {
	done   chan struct{}
	events chan audio.StreamEvent
	cancel context.CancelFunc
	users  int
	data   []byte
	err    error
}
type StreamingSynthesizer interface {
	SynthesizeStream(context.Context, string, func(audio.StreamEvent) error) error
}

func NewSpeechCache(voice Synthesizer, store AudioStore, name string, timeout ...time.Duration) *SpeechCache {
	limit := 120 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		limit = timeout[0]
	}
	return &SpeechCache{logger: zap.NewNop(), voice: voice, audio: store, name: name, timeout: limit, flights: map[string]*speechFlight{}}
}

// WithLogger sets the producer logger before the cache is used.
func (c *SpeechCache) WithLogger(logger *zap.Logger) *SpeechCache {
	if logger != nil {
		c.logger = logger
	}
	return c
}
func (c *SpeechCache) cacheKey(key string) string {
	sum := sha256.Sum256([]byte(c.name))
	return "geek/profiles/" + hex.EncodeToString(sum[:]) + "/" + key
}
func (c *SpeechCache) partKey(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return "geek/speech/" + hex.EncodeToString(sum[:]) + ".wav"
}

type warmSpeechContext struct{}

func (c *SpeechCache) WarmPart(ctx context.Context, text string) ([]byte, error) {
	return c.Part(context.WithValue(ctx, warmSpeechContext{}, true), text)
}
func (c *SpeechCache) Part(ctx context.Context, text string) ([]byte, error) {
	return c.Get(ctx, c.partKey(text), strings.TrimSpace(text))
}
func (c *SpeechCache) StreamPart(ctx context.Context, text string, emit func(audio.StreamEvent) error) error {
	return c.Stream(ctx, c.partKey(text), strings.TrimSpace(text), emit)
}
func (c *SpeechCache) acquire(ctx context.Context, key, text string, live bool) (*speechFlight, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.flights[key]; p != nil {
		p.users++
		return p, false
	}
	// Values survive a departing first listener; refcounts cancel user-only work.
	// The producer always has a total deadline, and a warmer remains a consumer
	// until its own context ends, preserving its shutdown/deadline independently.
	parent := context.WithoutCancel(ctx)
	if warm, _ := ctx.Value(warmSpeechContext{}).(bool); warm {
		parent = ctx
	}
	producer, cancel := context.WithTimeout(parent, c.timeout)
	p := &speechFlight{done: make(chan struct{}), cancel: cancel, users: 1}
	if live {
		p.events = make(chan audio.StreamEvent, 8)
	}
	c.flights[key] = p
	go func() {
		p.data, p.err = c.load(producer, key, text, live, func(e audio.StreamEvent) error {
			if !live {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case p.events <- e:
				return nil
			case <-ctx.Done():
				return nil
			case <-producer.Done():
				return producer.Err()
			case <-timer.C:
				cancel()
				return fmt.Errorf("stream drain timeout")
			}
		})
		cancel()
		c.mu.Lock()
		if c.flights[key] == p {
			delete(c.flights, key)
		}
		if p.events != nil {
			close(p.events)
		}
		close(p.done)
		c.mu.Unlock()
	}()
	return p, live
}
func (c *SpeechCache) release(key string, p *speechFlight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p.users--
	if p.users == 0 {
		p.cancel()
		if c.flights[key] == p {
			delete(c.flights, key)
		}
	}
}
func (c *SpeechCache) Get(ctx context.Context, key, text string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key = c.cacheKey(key)
	p, _ := c.acquire(ctx, key, text, false)
	defer c.release(key, p)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return p.data, p.err
	}
}
func (c *SpeechCache) Stream(ctx context.Context, key, text string, emit func(audio.StreamEvent) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key = c.cacheKey(key)
	liveCtx, stopLive := context.WithCancel(ctx)
	defer stopLive()
	p, live := c.acquire(liveCtx, key, text, true)
	defer c.release(key, p)
	c.logger.Info("geek stream producer", zap.Bool("live", live))
	if !live {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.done:
			if p.err != nil {
				return p.err
			}
			return audio.StreamWAV(p.data, emit)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-p.events:
			if !ok {
				<-p.done
				return p.err
			}
			if err := emit(e); err != nil {
				return err
			}
		}
	}
}
func (c *SpeechCache) load(ctx context.Context, key, text string, live bool, emit func(audio.StreamEvent) error) ([]byte, error) {
	if data, ok, err := c.audio.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		if _, _, err = audio.PCMFromWAV(data); err == nil {
			return data, audio.StreamWAV(data, emit)
		}
		c.logger.Warn("invalid geek audio cache object")
	}
	rate, seq := 0, 0
	pcm := audio.NewPCMBuffer()
	appendPCM := func(block []byte) error {
		if err := pcm.Append(block); err != nil {
			return err
		}
		for len(block) > 0 {
			n := min(len(block), audio.StreamBlockBytes)
			if err := emit(audio.StreamEvent{Type: "audio", Seq: seq, PCM: append([]byte(nil), block[:n]...)}); err != nil {
				return err
			}
			seq++
			block = block[n:]
		}
		return nil
	}
	for i, chunk := range SplitSpeech(text, speechLimit) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		partStarted, partEnded := false, false
		accept := func(e audio.StreamEvent) error {
			switch e.Type {
			case "start":
				if partStarted || e.SampleRate < 8000 || e.SampleRate > 192000 || e.Version != 1 || e.Channels != 1 || e.Encoding != "pcm_s16le" {
					return audio.ErrStream
				}
				partStarted = true
				if rate == 0 {
					rate = e.SampleRate
					return emit(audio.StartEvent(rate))
				}
				if rate != e.SampleRate {
					return audio.ErrStream
				}
				if i > 0 {
					return appendPCM(make([]byte, rate*150/1000*2))
				}
			case "audio":
				if !partStarted || partEnded {
					return audio.ErrStream
				}
				return appendPCM(e.PCM)
			case "end":
				if !partStarted || partEnded {
					return audio.ErrStream
				}
				partEnded = true
			default:
				return audio.ErrStream
			}
			return nil
		}
		var err error
		if stream, ok := c.voice.(StreamingSynthesizer); ok && live {
			err = stream.SynthesizeStream(ctx, chunk, accept)
		} else {
			var wav []byte
			wav, err = c.voice.Synthesize(ctx, chunk)
			if err == nil {
				err = audio.StreamWAV(wav, accept)
			}
		}
		if err != nil {
			return nil, err
		}
		if !partEnded {
			return nil, audio.ErrStream
		}
	}
	data, err := pcm.WAV(rate)
	if err != nil {
		return nil, fault.New(fault.Upstream, "SYNTHESIS_FAILED", "Сервис синтеза вернул несовместимые части.")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = c.audio.Put(ctx, key, data); err != nil {
		c.logger.Warn("geek audio cache write failure", zap.Error(err))
	}
	if err = emit(audio.StreamEvent{Type: "end", Chunks: seq, Samples: pcm.Samples()}); err != nil {
		return nil, err
	}
	return data, nil
}
