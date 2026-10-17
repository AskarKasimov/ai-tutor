package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Verify that the result is exactly the source minus the audited character spans.
// Python offsets count Unicode code points, so use runes rather than byte offsets.
func validCleaningResult(source string, result CleaningResult) bool {
	var report struct {
		Version      string `json:"version"`
		SourceSHA256 string `json:"source_sha256"`
		Removed      []struct {
			Start, End int
			Text       string
		} `json:"removed"`
	}
	if json.Unmarshal(result.Report, &report) != nil || report.Version != "material-cleaning-v1" {
		return false
	}
	hash := sha256.Sum256([]byte(source))
	if report.SourceSHA256 != hex.EncodeToString(hash[:]) {
		return false
	}
	original := []rune(source)
	var expected strings.Builder
	cursor := 0
	for _, block := range report.Removed {
		if block.Start < cursor || block.End <= block.Start || block.End > len(original) || string(original[block.Start:block.End]) != block.Text {
			return false
		}
		expected.WriteString(string(original[cursor:block.Start]))
		cursor = block.End
	}
	expected.WriteString(string(original[cursor:]))
	return expected.String() == result.Markdown
}
