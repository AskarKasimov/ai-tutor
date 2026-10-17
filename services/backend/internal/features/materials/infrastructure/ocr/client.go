package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	courseGraphURL string
	httpClient     *http.Client
}

type ConvertResponse struct {
	Markdown string          `json:"markdown"`
	Error    string          `json:"error,omitempty"`
	Report   json.RawMessage `json:"report"`
}

func New(courseGraphURL string, timeout time.Duration) *Client {
	return &Client{
		courseGraphURL: strings.TrimRight(courseGraphURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Convert converts the given document (PDF/Markdown/text) into structured Markdown with LaTeX math via course-graph.
func (c *Client) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	text, _, err := c.ConvertDetailed(ctx, filename, data)
	return text, err
}

func (c *Client) ConvertDetailed(ctx context.Context, filename string, data []byte) (string, json.RawMessage, error) {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return sanitizeUTF8(string(data)), json.RawMessage(`{"engine":"text","warnings":[]}`), nil
	}

	if c.courseGraphURL == "" {
		return "", nil, errors.New("сервис course-graph не настроен")
	}

	return c.convertViaCourseGraph(ctx, "", filename, data)
}

func (c *Client) ConvertJob(ctx context.Context, jobID, filename string, data []byte) (string, error) {
	text, _, err := c.ConvertJobDetailed(ctx, jobID, filename, data)
	return text, err
}

func (c *Client) ConvertJobDetailed(ctx context.Context, jobID, filename string, data []byte) (string, json.RawMessage, error) {
	if strings.TrimSpace(jobID) == "" {
		return "", nil, errors.New("OCR job ID is required")
	}
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return sanitizeUTF8(string(data)), json.RawMessage(`{"engine":"text","warnings":[]}`), nil
	}
	return c.convertViaCourseGraph(ctx, jobID, filename, data)
}

func (c *Client) Ready(ctx context.Context) error {
	if c.courseGraphURL == "" {
		return errors.New("сервис course-graph не настроен")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.courseGraphURL+"/ready/models", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var state struct {
		Ready bool   `json:"ready"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || !state.Ready {
		return fmt.Errorf("course-graph models are %s", state.State)
	}
	return nil
}

func (c *Client) convertViaCourseGraph(ctx context.Context, jobID, filename string, data []byte) (string, json.RawMessage, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", nil, fmt.Errorf("write form data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", nil, fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.courseGraphURL+"/ocr/convert", &body)
	if err != nil {
		return "", nil, fmt.Errorf("create OCR request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if jobID != "" {
		req.Header.Set("X-Job-ID", jobID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("обращение к сервису course-graph: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("сервис course-graph вернул статус %d: %s", resp.StatusCode, string(respBody))
	}

	var result ConvertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("decode OCR response: %w", err)
	}
	if result.Error != "" {
		return "", result.Report, fmt.Errorf("ошибка распознавания: %s", result.Error)
	}
	var report struct {
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(result.Report, &report)
	if len(report.Warnings) > 0 {
		// Service.Upload preserves a partial result and records its warning;
		// successful HTTP conversion must not hide missing image/formula content.
		return sanitizeUTF8(result.Markdown), result.Report, fmt.Errorf("Предупреждение OCR: %s", strings.Join(report.Warnings, "; "))
	}
	return sanitizeUTF8(result.Markdown), result.Report, nil
}

func sanitizeUTF8(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.ToValidUTF8(s, "")
}
