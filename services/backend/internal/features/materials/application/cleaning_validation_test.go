package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestCleaningMustMatchAuditedUnicodeSpans(t *testing.T) {
	source := "Теория\n\nСпасибо!\n\n$x^2$"
	hash := sha256.Sum256([]byte(source))
	report, _ := json.Marshal(map[string]any{"version": "material-cleaning-v1", "source_sha256": hex.EncodeToString(hash[:]), "removed": []map[string]any{{"start": 8, "end": 16, "text": "Спасибо!"}}})
	result := CleaningResult{Markdown: "Теория\n\n\n\n$x^2$", Report: report}
	if !validCleaningResult(source, result) {
		t.Fatal("valid Unicode span rejected")
	}
	result.Markdown = "Пересказ"
	if validCleaningResult(source, result) {
		t.Fatal("rewritten lecture accepted")
	}
	result.Markdown = "Теория"
	if validCleaningResult(source, result) {
		t.Fatal("unaudited deletion accepted")
	}
	if validCleaningResult("Другой источник", result) {
		t.Fatal("wrong source accepted")
	}
}
