package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

type TTSOptions struct {
	Seed               int64   `json:"seed"`
	CFGValue           float64 `json:"cfg_value"`
	InferenceTimesteps int     `json:"inference_timesteps"`
}

type Client struct {
	client         *http.Client
	sttURL, ttsURL string
	timeout        time.Duration
	ttsOptions     TTSOptions
}

func New(client *http.Client, sttURL, ttsURL string, timeout time.Duration, ttsOptions TTSOptions) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{client: client, sttURL: sttURL, ttsURL: ttsURL, timeout: timeout, ttsOptions: ttsOptions}
}

func providerError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return fault.New(fault.Timeout, "PROCESSING_TIMEOUT", "Превышено время ожидания голосового сервиса.")
	}
	return fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Голосовой сервис недоступен.")
}
func providerStatus(status int, failureCode string, stt bool) error {
	switch {
	case status == 503 || status == 429:
		return fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Голосовой сервис временно недоступен.")
	case status == 504 || status == 408:
		return fault.New(fault.Timeout, "PROCESSING_TIMEOUT", "Превышено время ожидания голосового сервиса.")
	case stt && status == 422:
		return fault.New(fault.Invalid, "INVALID_AUDIO", "Запись не удалось декодировать.")
	case stt && status == 415:
		return fault.New(fault.Unsupported, "UNSUPPORTED_AUDIO_FORMAT", "Неподдерживаемый формат записи.")
	case stt && status == 413:
		return fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Файл превышает 25 МиБ.")
	default:
		return fault.New(fault.Upstream, failureCode, "Голосовой сервис не вернул корректный результат.")
	}
}

func (c *Client) Recognize(ctx context.Context, data []byte, media string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="audio"`)
	h.Set("Content-Type", media)
	part, err := writer.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", c.sttURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.client.Do(req)
	if err != nil {
		return "", providerError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", providerStatus(resp.StatusCode, "TRANSCRIPTION_FAILED", true)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil {
		return "", providerError(err)
	}
	var result struct {
		Text *string `json:"text"`
	}
	if len(content) > 8*1024*1024 || !utf8.Valid(content) || json.Unmarshal(content, &result) != nil || result.Text == nil || strings.IndexByte(*result.Text, 0) >= 0 {
		return "", fault.New(fault.Upstream, "TRANSCRIPTION_FAILED", "Некорректный ответ сервиса распознавания.")
	}
	return strings.TrimSpace(*result.Text), nil
}

func (c *Client) Synthesize(ctx context.Context, text string) ([]byte, error) {
	body, err := json.Marshal(struct {
		Text string `json:"text"`
		TTSOptions
	}{Text: text, TTSOptions: c.ttsOptions})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	providerReq, err := http.NewRequestWithContext(ctx, "POST", c.ttsURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	providerReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(providerReq)
	if err != nil {
		return nil, providerError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, providerStatus(resp.StatusCode, "SYNTHESIS_FAILED", false)
	}
	// Bound unexpected provider responses; 64 MiB exceeds a 500-character WAV.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024+1))
	if err != nil {
		return nil, providerError(err)
	}
	if len(data) > 64*1024*1024 || !audio.ValidWAV(data) {
		return nil, fault.New(fault.Upstream, "SYNTHESIS_FAILED", "Сервис синтеза не вернул корректный WAV.")
	}
	return data, nil
}
