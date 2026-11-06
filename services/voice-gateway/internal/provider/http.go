// Package provider implements the existing GigaAM and VoxCPM HTTP APIs.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc/codes"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/voice-gateway/internal/audio"
)

// Failure contains a safe category, never an upstream response body or URL.
type Failure struct {
	Code      codes.Code
	Uncertain bool
}

func (e *Failure) Error() string { return "voice upstream failure: " + e.Code.String() }
func transportFailure(err error) error {
	var op *net.OpError
	beforeSend := errors.As(err, &op) && op.Op == "dial"
	code := codes.Unavailable
	if errors.Is(err, context.DeadlineExceeded) {
		code = codes.DeadlineExceeded
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		code = codes.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		code = codes.Canceled
	}
	return &Failure{Code: code, Uncertain: !beforeSend}
}
func responseFailure(resp *http.Response, stt bool) error {
	code := codes.Internal
	switch resp.StatusCode {
	case 429, 503:
		code = codes.Unavailable
	case 408, 504:
		code = codes.DeadlineExceeded
	case 413:
		if stt {
			code = codes.OutOfRange
		}
	case 415:
		if stt {
			code = codes.Unimplemented
		}
	case 422:
		if stt {
			code = codes.InvalidArgument
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			var body struct {
				Detail string `json:"detail"`
			}
			if json.Unmarshal(data, &body) == nil && strings.HasPrefix(body.Detail, "Unsupported ASR model:") {
				code = codes.Unavailable
			}
		}
	}
	return &Failure{Code: code, Uncertain: resp.StatusCode == http.StatusGatewayTimeout || resp.StatusCode == http.StatusRequestTimeout}
}

type STT struct {
	URL      string
	Model    string
	Longform bool
	Timeout  time.Duration
}
type TTS struct {
	URL      string
	Timeout  time.Duration
	Profiles map[string]Options
}
type Options struct {
	Seed               int64   `json:"seed"`
	CFGValue           float64 `json:"cfg_value"`
	InferenceTimesteps int     `json:"inference_timesteps"`
	UseReference       bool    `json:"use_reference"`
	UsePrompt          bool    `json:"use_prompt"`
}
type HTTP struct {
	Client *http.Client
	STT    map[string]STT
	TTS    map[string]TTS
}

func (p *HTTP) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p *HTTP) Transcribe(ctx context.Context, instance string, data []byte, media string) (string, error) {
	cfg, ok := p.STT[instance]
	if !ok {
		return "", errors.New("unknown STT instance")
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	parts := [][]byte{data}
	base, _, _ := mime.ParseMediaType(media)
	if !cfg.Longform && (base == "audio/wav" || base == "audio/x-wav" || base == "audio/wave") {
		if split, ok := audio.SplitWAV(data, 20); ok {
			parts = split
		}
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		text, err := p.transcribeOnce(ctx, cfg, part, media)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(text) != "" {
			texts = append(texts, strings.TrimSpace(text))
		}
	}
	return strings.Join(texts, " "), nil
}
func (p *HTTP) transcribeOnce(ctx context.Context, cfg STT, data []byte, media string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="audio"`)
	header.Set("Content-Type", media)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	target, err := url.Parse(cfg.URL)
	if err != nil {
		return "", err
	}
	if cfg.Model != "" {
		query := target.Query()
		query.Set("model", cfg.Model)
		target.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := p.client().Do(req)
	if err != nil {
		return "", transportFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", responseFailure(response, true)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil {
		return "", err
	}
	var parsed struct {
		Text *string `json:"text"`
	}
	if len(payload) > 8*1024*1024 || !utf8.Valid(payload) || json.Unmarshal(payload, &parsed) != nil || parsed.Text == nil || strings.ContainsRune(*parsed.Text, 0) {
		return "", &Failure{Code: codes.Internal}
	}
	return *parsed.Text, nil
}
func (p *HTTP) Synthesize(ctx context.Context, instance, profile, text string) ([]byte, error) {
	cfg, ok := p.TTS[instance]
	if !ok {
		return nil, errors.New("unknown TTS instance")
	}
	opts, ok := cfg.Profiles[profile]
	if !ok {
		return nil, errors.New("unknown voice profile")
	}
	if !strings.HasSuffix(text, "!!!") {
		text += "!!!"
	}
	payload, err := json.Marshal(struct {
		Text string `json:"text"`
		Options
	}{Text: text, Options: opts})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client().Do(req)
	if err != nil {
		return nil, transportFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, responseFailure(response, false)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024*1024 || !audio.ValidWAV(data) {
		return nil, &Failure{Code: codes.Internal}
	}
	return data, nil
}
