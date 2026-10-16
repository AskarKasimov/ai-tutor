// Copy into services/backend/cmd/translationqa/main.go at the evaluated commit.
// Uses the actual translator package unchanged, including prompt and HTTP path.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/features/materials/infrastructure/translator"
	"os"
	"strings"
	"time"
)

type Topic struct {
	ID        string  `json:"id"`
	Text      string  `json:"full_text"`
	Reference *string `json:"full_text_ru"`
}
type Result struct {
	ID               string  `json:"id"`
	Translation      string  `json:"full_text_ru"`
	Error            string  `json:"error,omitempty"`
	Elapsed          float64 `json:"elapsed_seconds"`
	Chunks           int     `json:"chunks"`
	MaxChunkBytes    int     `json:"max_chunk_bytes"`
	NeedsTranslation bool    `json:"needs_translation"`
}

func main() {
	input := flag.String("input", "", "localized dataset")
	output := flag.String("output", "", "append-only results JSONL")
	limit := flag.Int("limit", 0, "maximum reference topics, 0 = all")
	ids := flag.String("ids", "", "optional comma-separated topic IDs")
	flag.Parse()
	raw, err := os.ReadFile(*input)
	if err != nil {
		panic(err)
	}
	var graph struct {
		Topics []Topic `json:"topics"`
	}
	if err = json.Unmarshal(raw, &graph); err != nil {
		panic(err)
	}
	// Resume successful or failed attempts without silently retrying or overwriting.
	done := map[string]bool{}
	if f, err := os.Open(*output); err == nil {
		dec := json.NewDecoder(f)
		for {
			var r Result
			if dec.Decode(&r) != nil {
				break
			}
			done[r.ID] = true
		}
		f.Close()
	}
	f, err := os.OpenFile(*output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	client := translator.New(nil, os.Getenv("QA_TRANSLATOR_URL"), os.Getenv("QA_TRANSLATOR_MODEL"), os.Getenv("QA_TRANSLATOR_KEY"), 4, 120*time.Second)
	count := 0
	selected := map[string]bool{}
	if *ids != "" {
		for _, id := range strings.Split(*ids, ",") {
			selected[id] = true
		}
	}
	for _, t := range graph.Topics {
		if len(selected) > 0 && !selected[t.ID] {
			continue
		}
		if t.Reference == nil || *t.Reference == "" || done[t.ID] {
			continue
		}
		if *limit > 0 && count >= *limit {
			break
		}
		count++
		r := Result{ID: t.ID, NeedsTranslation: translator.NeedsTranslation(t.Text)}
		chunks := translator.SplitIntoChunks(t.Text, translator.DefaultMaxCharsPerChunk)
		r.Chunks = len(chunks)
		for _, c := range chunks {
			if len(c.Text) > r.MaxChunkBytes {
				r.MaxChunkBytes = len(c.Text)
			}
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		r.Translation, err = client.Translate(ctx, t.Text)
		cancel()
		r.Elapsed = time.Since(start).Seconds()
		if err != nil {
			r.Error = err.Error()
		}
		if err = enc.Encode(r); err != nil {
			panic(err)
		}
		f.Sync()
		fmt.Printf("%s success=%t chunks=%d seconds=%.1f\n", r.ID, r.Error == "", r.Chunks, r.Elapsed)
		// Stop on infrastructure failure instead of issuing an entire failing batch.
		if r.Error != "" {
			os.Exit(1)
		}
	}
}
