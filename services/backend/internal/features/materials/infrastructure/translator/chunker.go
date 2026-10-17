package translator

import (
	"strings"
	"unicode/utf8"
)

// DefaultMaxCharsPerChunk is the target maximum size of a text chunk in characters (~500-800 tokens).
const DefaultMaxCharsPerChunk = 2500

// Chunk represents an indexed portion of markdown text for parallel translation.
type Chunk struct {
	Index     int
	Text      string
	separator *string
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
		// Split large sections at paragraph/line/word boundaries. Continuations
		// retain their separator so splitting a Markdown table does not add blank rows.
		if len(section) > maxCharsPerChunk {
			if currentBuilder.Len() > 0 {
				chunks = append(chunks, Chunk{
					Index: chunkIndex,
					Text:  strings.TrimSpace(currentBuilder.String()),
				})
				chunkIndex++
				currentBuilder.Reset()
			}
			for _, fragment := range splitLargeSection(section, maxCharsPerChunk) {
				fragment.Index = chunkIndex
				chunks = append(chunks, fragment)
				chunkIndex++
			}
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
	separators := make([]*string, len(chunks))
	for _, c := range chunks {
		if c.Index >= 0 && c.Index < len(chunks) {
			ordered[c.Index] = strings.TrimSpace(c.Text)
			separators[c.Index] = c.separator
		}
	}

	var sb strings.Builder
	for i, text := range ordered {
		if text == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			if separators[i] == nil {
				sb.WriteString("\n\n")
			} else {
				sb.WriteString(*separators[i])
			}
		}
		sb.WriteString(text)
	}

	return sb.String()
}

func splitLargeSection(text string, limit int) []Chunk {
	var result []Chunk
	var before *string
	for len(text) > limit {
		cut, separator := strings.LastIndex(text[:limit], "\n\n"), "\n\n"
		if cut < 1 {
			cut, separator = strings.LastIndex(text[:limit], "\n"), "\n"
		}
		if cut < 1 && strings.HasPrefix(strings.TrimSpace(text), "|") {
			// A table row is indivisible; keep its cells together even if one
			// unusually large row exceeds the target size.
			cut, separator = strings.Index(text, "\n"), "\n"
			if cut < 1 {
				break
			}
		}
		if cut < 1 {
			cut, separator = strings.LastIndex(text[:limit], " "), " "
		}
		if cut < 1 {
			cut, separator = limit, ""
			for cut > 0 && !utf8.RuneStart(text[cut]) {
				cut--
			}
			if cut == 0 {
				_, cut = utf8.DecodeRuneInString(text)
			}
		}
		result = append(result, Chunk{Text: text[:cut], separator: before})
		text = text[cut+len(separator):]
		joiner := separator
		before = &joiner
	}
	if text != "" {
		result = append(result, Chunk{Text: text, separator: before})
	}
	return result
}
