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
	Markdown string `json:"markdown"`
	Error    string `json:"error,omitempty"`
	Report   struct {
		Warnings []string `json:"warnings"`
	} `json:"report"`
}

func New(courseGraphURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Client{
		courseGraphURL: strings.TrimRight(courseGraphURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Convert converts the given document (PDF/Markdown/text) into structured Markdown with LaTeX math via course-graph.
func (c *Client) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return sanitizeUTF8(string(data)), nil
	}

	if c.courseGraphURL == "" {
		return "", errors.New("сервис course-graph не настроен")
	}

	return c.convertViaCourseGraph(ctx, filename, data)
}

func (c *Client) convertViaCourseGraph(ctx context.Context, filename string, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write form data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.courseGraphURL+"/ocr/convert", &body)
	if err != nil {
		return "", fmt.Errorf("create OCR request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("обращение к сервису course-graph: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("сервис course-graph вернул статус %d: %s", resp.StatusCode, string(respBody))
	}

	var result ConvertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode OCR response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("ошибка распознавания: %s", result.Error)
	}
	if len(result.Report.Warnings) > 0 {
		// Service.Upload preserves a partial result and records its warning;
		// successful HTTP conversion must not hide missing image/formula content.
		return sanitizeUTF8(result.Markdown), fmt.Errorf("Предупреждение OCR: %s", strings.Join(result.Report.Warnings, "; "))
	}
	return sanitizeUTF8(result.Markdown), nil
}

func sanitizeUTF8(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.ToValidUTF8(s, "")
}
