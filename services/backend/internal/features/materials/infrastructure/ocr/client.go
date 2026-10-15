package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	ocrURL     string
	httpClient *http.Client
}

type ConvertResponse struct {
	Markdown string `json:"markdown"`
	Error    string `json:"error,omitempty"`
}

func New(ocrURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Client{
		ocrURL: strings.TrimRight(ocrURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Convert converts the given document (PDF/Markdown/text) into structured Markdown with LaTeX math.
func (c *Client) Convert(ctx context.Context, filename string, data []byte) (string, error) {
	// If OCR service is configured, try it first
	if c.ocrURL != "" {
		md, err := c.convertViaOCRService(ctx, filename, data)
		if err == nil && len(strings.TrimSpace(md)) > 0 {
			return md, nil
		}
		log.Printf("OCR service (%s) warning for %s: %v, falling back to local extraction", c.ocrURL, filename, err)
	}

	// Fallback to local extraction
	return c.convertLocalFallback(ctx, filename, data)
}

func (c *Client) convertViaOCRService(ctx context.Context, filename string, data []byte) (string, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ocrURL+"/convert", &body)
	if err != nil {
		return "", fmt.Errorf("create OCR request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send OCR request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OCR service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ConvertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode OCR response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("OCR error: %s", result.Error)
	}
	return sanitizeUTF8(result.Markdown), nil
}

func (c *Client) convertLocalFallback(ctx context.Context, filename string, data []byte) (string, error) {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return sanitizeUTF8(string(data)), nil
	}

	if strings.HasSuffix(lower, ".pdf") {
		pdftotextPath, err := exec.LookPath("pdftotext")
		if err != nil {
			return "", fmt.Errorf("pdftotext is not available on host and OCR service is unreachable: %w", err)
		}

		tmpFile, err := os.CreateTemp("", "doc-*.pdf")
		if err != nil {
			return "", fmt.Errorf("create temp pdf: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return "", fmt.Errorf("write temp pdf: %w", err)
		}
		tmpFile.Close()

		cmd := exec.CommandContext(ctx, pdftotextPath, tmpFile.Name(), "-")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("pdftotext extraction: %w (%s)", err, stderr.String())
		}
		return sanitizeUTF8(stdout.String()), nil
	}

	return "", fmt.Errorf("unsupported file format %q for text extraction without OCR service", filename)
}

func sanitizeUTF8(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.ToValidUTF8(s, "")
}
