package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"go.uber.org/zap"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// SynthesizeStream validates each provider frame before handing it to consumers.
// Only unavailable endpoint statuses fall back to WAV, before any frame is emitted.
func (c *Client) SynthesizeStream(ctx context.Context, text string, emit func(audio.StreamEvent) error) (err error) {
	started := time.Now()
	first := true
	defer func() {
		logCall(ctx, "tts_stream", started, []zap.Field{zap.Int("text_chars", utf8.RuneCountInString(text)), zap.Int("inference_timesteps", c.ttsOptions.InferenceTimesteps)}, err)
	}()
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if !strings.HasSuffix(text, "!!!") {
		text += "!!!"
	}
	body, err := json.Marshal(struct {
		Text string `json:"text"`
		TTSOptions
	}{text, c.ttsOptions})
	if err != nil {
		return err
	}
	endpoint, err := url.Parse(c.ttsURL)
	if err != nil {
		return err
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/stream"
	endpoint.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := c.client.Do(req)
	if err != nil {
		return providerError(err)
	}
	if resp.StatusCode == 404 || resp.StatusCode == 406 || resp.StatusCode == 501 {
		resp.Body.Close()
		wav, err := c.synthesize(ctx, text)
		if err != nil {
			return err
		}
		return audio.StreamWAV(wav, emit)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return statusError(resp, "SYNTHESIS_FAILED", false)
	}
	media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if media != "application/x-ndjson" {
		return fmt.Errorf("%w: provider stream content type", audio.ErrStream)
	}
	err = audio.ParseStream(ctx, resp.Body, func(e audio.StreamEvent) error {
		if e.Type == "audio" && first {
			first = false
			httpLogger := []zap.Field{zap.Duration("ttfa", time.Since(started))}
			logCall(ctx, "tts_stream_first_audio", started, httpLogger, nil)
		}
		return emit(e)
	})
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
