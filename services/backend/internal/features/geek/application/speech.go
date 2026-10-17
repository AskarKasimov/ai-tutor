package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

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

// SpeechCache synthesizes texts within the TTS limit and keeps the WAV in the
// audio store. It is shared by the turns students listen to and by the warmer
// that voices prepared questions ahead of them.
type SpeechCache struct {
	voice Synthesizer
	audio AudioStore
	// name identifies the TTS settings; a part's key depends on it, so new
	// settings never return audio made with the old ones.
	name string
}

func NewSpeechCache(voice Synthesizer, audio AudioStore, name string) *SpeechCache {
	return &SpeechCache{voice: voice, audio: audio, name: name}
}

// Part voices a turn part, cached by its text, so the same prepared question
// is synthesized once for every student.
func (c *SpeechCache) Part(ctx context.Context, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	sum := sha256.Sum256([]byte(c.name + "\n" + text))
	return c.Get(ctx, "geek/speech/"+hex.EncodeToString(sum[:])+".wav", text)
}

// Get returns the WAV cached under key or synthesizes text by parts within
// the TTS limit, joins and caches it.
func (c *SpeechCache) Get(ctx context.Context, key, text string) ([]byte, error) {
	if data, ok, err := c.audio.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return data, nil
	}
	parts := [][]byte{}
	for _, chunk := range SplitSpeech(text, speechLimit) {
		data, err := c.voice.Synthesize(ctx, chunk)
		if err != nil {
			return nil, err
		}
		parts = append(parts, data)
	}
	data, ok := audio.JoinWAV(parts)
	if !ok {
		return nil, fault.New(fault.Upstream, "SYNTHESIS_FAILED", "Сервис синтеза вернул несовместимые части.")
	}
	if err := c.audio.Put(ctx, key, data); err != nil {
		return nil, err
	}
	return data, nil
}
