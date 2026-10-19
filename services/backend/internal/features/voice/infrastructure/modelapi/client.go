package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/audio"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
	"go.uber.org/zap"
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

// providerError keeps the transport cause next to the fault, so the request
// log tells a refused connection from a reset or a timeout.
func providerError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return errors.Join(fault.New(fault.Timeout, "PROCESSING_TIMEOUT", "Превышено время ожидания голосового сервиса."), err)
	}
	return errors.Join(fault.New(fault.Unavailable, "PROCESSING_UNAVAILABLE", "Голосовой сервис недоступен."), err)
}

// statusError is the fault for a provider status with the start of its body.
func statusError(resp *http.Response, failureCode string, stt bool) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return errors.Join(providerStatus(resp.StatusCode, failureCode, stt),
		fmt.Errorf("provider answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
}

// logCall records one provider request: how long it took, how much was sent
// and received, and why it failed. Slow synthesis is the usual reason a Geek
// turn is voiced late.
func logCall(ctx context.Context, operation string, started time.Time, fields []zap.Field, err error) {
	fields = append(fields, zap.String("voice_operation", operation), zap.Duration("duration", time.Since(started)))
	logger := httpx.LoggerFromContext(ctx)
	switch {
	case err == nil:
		logger.Info("voice call", fields...)
	case errors.Is(err, context.Canceled):
		logger.Info("voice call cancelled by the client", fields...)
	default:
		logger.Warn("voice call failed", append(fields, zap.Error(err))...)
	}
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

// sttPartSeconds keeps each request within the short-form limit of GigaAM:
// without its long-form VAD weights the model rejects audio over ~25 s.
const sttPartSeconds = 20

// Recognize transcribes the audio. A long 16-bit PCM WAV is sent in parts cut
// at pauses, and their texts are joined.
func (c *Client) Recognize(ctx context.Context, data []byte, media string) (string, error) {
	base, _, _ := mime.ParseMediaType(media)
	if base != "audio/wav" && base != "audio/x-wav" && base != "audio/wave" {
		return c.recognize(ctx, data, media)
	}
	parts, ok := audio.SplitWAV(data, sttPartSeconds)
	if !ok || len(parts) == 1 {
		return c.recognize(ctx, data, media)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		text, err := c.recognize(ctx, part, "audio/wav")
		if err != nil {
			return "", err
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, " "), nil
}

func (c *Client) recognize(ctx context.Context, data []byte, media string) (text string, err error) {
	defer func(started time.Time) {
		logCall(ctx, "stt", started, []zap.Field{zap.String("media", media), zap.Int("audio_bytes", len(data)), zap.Int("text_chars", utf8.RuneCountInString(text))}, err)
	}(time.Now())
	return c.recognizeOnce(ctx, data, media)
}

func (c *Client) recognizeOnce(ctx context.Context, data []byte, media string) (string, error) {
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
		return "", statusError(resp, "TRANSCRIPTION_FAILED", true)
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

func (c *Client) Synthesize(ctx context.Context, text string) (data []byte, err error) {
	defer func(started time.Time) {
		logCall(ctx, "tts", started, []zap.Field{zap.Int("text_chars", utf8.RuneCountInString(text)), zap.Int("audio_bytes", len(data)),
			zap.Int("inference_timesteps", c.ttsOptions.InferenceTimesteps)}, err)
	}(time.Now())
	return c.synthesize(ctx, text)
}

func (c *Client) synthesize(ctx context.Context, text string) ([]byte, error) {
	const calmnessPrefix = "(calm, composed, friendly tone; speak evenly, without irritation or anger) "
	const completionSuffix = "!!!"
	text = calmnessPrefix + text
	if !strings.HasSuffix(text, completionSuffix) {
		text += completionSuffix
	}
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
		return nil, statusError(resp, "SYNTHESIS_FAILED", false)
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
