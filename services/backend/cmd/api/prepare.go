package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/exam"
	geekapp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/application"
	geekllm "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/geek/infrastructure/llm"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/httpx"
)

// prepareQuestions fills the missing main questions of a graph file through
// the LLM gateway (BACKEND_LLM_GATEWAY_ADDR, BACKEND_LLM_TIMEOUT) and writes
// the result. Nodes that failed are written without a question and reported.
func prepareQuestions(ctx context.Context, logger *zap.Logger, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: api prepare-questions <graph.json> <out.json>")
	}
	address := strings.TrimSpace(os.Getenv("BACKEND_LLM_GATEWAY_ADDR"))
	if address == "" {
		return fmt.Errorf("BACKEND_LLM_GATEWAY_ADDR is required")
	}
	timeout := 120 * time.Second
	if value := strings.TrimSpace(os.Getenv("BACKEND_LLM_TIMEOUT")); value != "" {
		var err error
		if timeout, err = time.ParseDuration(value); err != nil {
			return fmt.Errorf("BACKEND_LLM_TIMEOUT: %w", err)
		}
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	var file struct {
		Nodes []exam.Node `json:"nodes"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("decode graph: %w", err)
	}
	graph := exam.Graph{Nodes: file.Nodes}
	if err := graph.Validate(); err != nil {
		return err
	}
	examiner, conn, err := geekllm.Dial(address, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	started := time.Now()
	prepared, count, failures := geekapp.PrepareQuestions(httpx.WithLogger(ctx, logger), examiner, graph, 4)
	file.Nodes = prepared.Nodes
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(file); err != nil {
		return err
	}
	if err := os.WriteFile(args[1], out.Bytes(), 0o644); err != nil {
		return err
	}
	logger.Info("questions prepared", zap.Int("prepared", count), zap.Int("nodes", len(graph.Nodes)), zap.Duration("duration", time.Since(started)), zap.Error(failures))
	return failures
}
