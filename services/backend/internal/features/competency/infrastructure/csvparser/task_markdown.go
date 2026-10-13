package csvparser

import (
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const taskBlockSeparator = "\n\n\n"

// Four positional blocks: question, options, voice instruction, reference answer.
// Two blank lines are reserved for the separator, including inside code blocks.
func parseMLTask(raw string) (competencymap.Task, bool) {
	var task competencymap.Task
	blocks := strings.Split(strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n")), taskBlockSeparator)
	if len(blocks) != 4 {
		return task, false
	}
	for i := range blocks {
		blocks[i] = strings.TrimSpace(blocks[i])
		if blocks[i] == "" {
			return task, false
		}
	}
	options, ok := parseMarkdownOptions(blocks[1])
	if !ok {
		return task, false
	}
	task.Question, task.Options, task.VoiceInstruction, task.ReferenceAnswer = blocks[0], options, blocks[2], blocks[3]
	return task, true
}

// Goldmark determines list item boundaries. Slice the original source rather
// than rendering the AST, so formatting, fences and escapes remain Markdown.
func parseMarkdownOptions(raw string) ([]string, bool) {
	if raw == "[]" {
		return []string{}, true
	}
	source := []byte(raw)
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	list, ok := document.FirstChild().(*ast.List)
	if !ok || list.NextSibling() != nil {
		return nil, false
	}
	var items []*ast.ListItem
	var starts []int
	for node := list.FirstChild(); node != nil; node = node.NextSibling() {
		item := node.(*ast.ListItem)
		first := item.FirstChild()
		// An option starts with text; further paragraphs, code and nested lists
		// belong to the same option.
		if first == nil || (first.Kind() != ast.KindTextBlock && first.Kind() != ast.KindParagraph) || first.Lines().Len() == 0 {
			return nil, false
		}
		start := first.Lines().At(0).Start
		lineStart := strings.LastIndex(raw[:start], "\n") + 1
		// Require text on the marker line so its source range begins here.
		marker := strings.TrimSpace(raw[lineStart:start])
		if marker == "" || marker[len(marker)-1] != list.Marker {
			return nil, false
		}
		items = append(items, item)
		starts = append(starts, start)
	}
	options := make([]string, 0, len(items))
	for i, item := range items {
		end := len(source)
		if i+1 < len(items) {
			end = strings.LastIndex(raw[:starts[i+1]], "\n") + 1
		}
		lines := strings.Split(raw[starts[i]:end], "\n")
		for j := 1; j < len(lines); j++ {
			pos, padding := util.IndentPosition([]byte(lines[j]), 0, item.Offset)
			if pos >= 0 {
				lines[j] = strings.Repeat(" ", padding) + lines[j][pos:]
			}
		}
		option := strings.TrimSpace(strings.Join(lines, "\n"))
		if option == "" {
			return nil, false
		}
		options = append(options, option)
	}
	return options, len(options) > 0
}

// taskFragmentCode explains why a nonempty cell was not imported as a task.
func taskFragmentCode(raw string) string {
	blocks := strings.Split(strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n")), taskBlockSeparator)
	if len(blocks) != 4 {
		return "TASK_INVALID_BLOCK_COUNT"
	}
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			return "TASK_EMPTY_PART"
		}
	}
	if _, ok := parseMarkdownOptions(strings.TrimSpace(blocks[1])); !ok {
		return "TASK_INVALID_OPTIONS"
	}
	return "TASK_CELL_FRAGMENT"
}
