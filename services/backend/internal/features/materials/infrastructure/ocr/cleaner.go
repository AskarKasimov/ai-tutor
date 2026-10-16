package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/application"
)

func (c *Client) Clean(ctx context.Context, markdown, materialID string) (application.CleaningResult, error) {
	var result application.CleaningResult
	payload, err := json.Marshal(map[string]string{"markdown": markdown, "material_id": materialID})
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.courseGraphURL+"/topics/extract", bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return result, fmt.Errorf("course-graph cleaning: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("course-graph cleaning HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 32*1024*1024)).Decode(&result)
	return result, err
}
