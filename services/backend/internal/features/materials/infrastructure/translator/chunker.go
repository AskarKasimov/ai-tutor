package translator

import (
	"strings"
)

// DefaultMaxCharsPerChunk is the target maximum size of a text chunk in characters (~500-800 tokens).
const DefaultMaxCharsPerChunk = 2500

// Chunk represents an indexed portion of markdown text for parallel translation.
type Chunk struct {
	Index int
	Text  string
}

// SplitIntoChunks slices a Markdown lecture into coherent chunks (usually 1-3 slides each)
// without breaking individual slides apart whenever possible.
func SplitIntoChunks(markdown string, maxCharsPerChunk int) []Chunk {
	if maxCharsPerChunk <= 0 {
		maxCharsPerChunk = DefaultMaxCharsPerChunk
	}

	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return nil
	}

	// 1. Identify natural sections by scanning lines for "# " or "## " headers
	lines := strings.Split(trimmed, "\n")
	var sections []string
	var currentSection strings.Builder

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		isHeader := strings.HasPrefix(trimmedLine, "## ") || (strings.HasPrefix(trimmedLine, "# ") && currentSection.Len() > 0)
		if isHeader && currentSection.Len() > 0 {
			sections = append(sections, strings.TrimSpace(currentSection.String()))
			currentSection.Reset()
		}
		if currentSection.Len() > 0 {
			currentSection.WriteByte('\n')
		}
		currentSection.WriteString(line)
	}

	if currentSection.Len() > 0 {
		sections = append(sections, strings.TrimSpace(currentSection.String()))
	}

	// If no headers found, fallback to paragraphs
	if len(sections) <= 1 && !strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "# ") {
		paragraphs := strings.Split(trimmed, "\n\n")
		var validParagraphs []string
		for _, p := range paragraphs {
			p = strings.TrimSpace(p)
			if p != "" {
				validParagraphs = append(validParagraphs, p)
			}
		}
		if len(validParagraphs) > 1 {
			sections = validParagraphs
		}
	}

	if len(sections) == 0 {
		return []Chunk{{Index: 0, Text: trimmed}}
	}

	// 2. Group sections into chunks of size <= maxCharsPerChunk
	var chunks []Chunk
	var currentBuilder strings.Builder
	chunkIndex := 0

	for _, section := range sections {
		// If a single section alone exceeds limit, flush current and append section alone
		if len(section) > maxCharsPerChunk {
			if currentBuilder.Len() > 0 {
				chunks = append(chunks, Chunk{
					Index: chunkIndex,
					Text:  strings.TrimSpace(currentBuilder.String()),
				})
				chunkIndex++
				currentBuilder.Reset()
			}
			chunks = append(chunks, Chunk{
				Index: chunkIndex,
				Text:  section,
			})
			chunkIndex++
			continue
		}

		if currentBuilder.Len() > 0 && currentBuilder.Len()+len(section)+2 > maxCharsPerChunk {
			chunks = append(chunks, Chunk{
				Index: chunkIndex,
				Text:  strings.TrimSpace(currentBuilder.String()),
			})
			chunkIndex++
			currentBuilder.Reset()
		}

		if currentBuilder.Len() > 0 {
			currentBuilder.WriteString("\n\n")
		}
		currentBuilder.WriteString(section)
	}

	if currentBuilder.Len() > 0 {
		chunks = append(chunks, Chunk{
			Index: chunkIndex,
			Text:  strings.TrimSpace(currentBuilder.String()),
		})
	}

	return chunks
}

// JoinChunks combines translated chunks preserving their sequence.
func JoinChunks(chunks []Chunk) string {
	if len(chunks) == 0 {
		return ""
	}

	ordered := make([]string, len(chunks))
	for _, c := range chunks {
		if c.Index >= 0 && c.Index < len(chunks) {
			ordered[c.Index] = strings.TrimSpace(c.Text)
		}
	}

	var sb strings.Builder
	for i, text := range ordered {
		if text == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(text)
	}

	return sb.String()
}
