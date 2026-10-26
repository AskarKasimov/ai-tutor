package csvparser

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/variant"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const taskBlockSeparator = "\n\n\n"

// maxVoiceInstruction is the longest instruction TTS and the diagnostic take.
const maxVoiceInstruction = 500

// The instructions a three-block task is voiced with when its question is
// too long to be read out.
const (
	optionsInstruction = "Назовите вариант ответа и кратко объясните свой выбор."
	openInstruction    = "Ответьте на вопрос и кратко объясните ответ."
)

// Four positional blocks: question, options, voice instruction, reference
// answer. Three blocks leave the instruction out: the question is voiced as
// on the screen, or a standard instruction when it is longer than TTS takes.
// Two blank lines are reserved for the separator, including inside code blocks.
func parseMLTask(raw string) (competencymap.Task, bool) {
	var task competencymap.Task
	blocks := splitTaskBlocks(raw)
	if len(blocks) != 3 && len(blocks) != 4 {
		return task, false
	}
	for i := range blocks {
		blocks[i] = strings.TrimSpace(blocks[i])
		if blocks[i] == "" {
			return task, false
		}
	}
	options, ok := parseTaskOptions(blocks[1])
	if !ok {
		return task, false
	}
	task.Question, task.Options, task.ReferenceAnswer = blocks[0], options, blocks[len(blocks)-1]
	if len(blocks) == 4 {
		task.VoiceInstruction = blocks[2]
	} else {
		task.VoiceInstruction = spokenQuestion(task.Question, len(options) > 0)
	}
	if !variant.ValidTaskContent(task.Question, task.Options, task.VoiceInstruction, task.ReferenceAnswer) {
		return competencymap.Task{}, false
	}
	return task, true
}

func splitTaskBlocks(raw string) []string {
	return strings.Split(strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n")), taskBlockSeparator)
}

// spokenQuestion is the instruction of a task without one: the question
// itself, or a standard instruction when TTS cannot take the question.
func spokenQuestion(question string, withOptions bool) string {
	if utf8.RuneCountInString(question) <= maxVoiceInstruction {
		return question
	}
	if withOptions {
		return optionsInstruction
	}
	return openInstruction
}

// letterOption is a line of a lettered option list: «A. …», «b) …», «Б. …».
var letterOption = regexp.MustCompile(`^[A-Za-zА-ЯЁа-яё][.)][ \t]+(\S.*)$`)

// parseTaskOptions reads a Markdown list or, failing that, options written
// one per line after a Latin or Cyrillic letter.
func parseTaskOptions(raw string) ([]string, bool) {
	if options, ok := parseMarkdownOptions(raw); ok {
		return options, true
	}
	return parseLetterOptions(raw)
}

func parseLetterOptions(raw string) ([]string, bool) {
	var options []string
	for _, line := range strings.Split(raw, "\n") {
		match := letterOption.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			return nil, false
		}
		options = append(options, strings.TrimSpace(match[1]))
	}
	return options, len(options) > 0
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
	blocks := splitTaskBlocks(raw)
	if len(blocks) != 3 && len(blocks) != 4 {
		return "TASK_INVALID_BLOCK_COUNT"
	}
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			return "TASK_EMPTY_PART"
		}
	}
	if _, ok := parseTaskOptions(strings.TrimSpace(blocks[1])); !ok {
		return "TASK_INVALID_OPTIONS"
	}
	voiceInstruction := spokenQuestion(strings.TrimSpace(blocks[0]), len(blocks) > 1)
	if len(blocks) == 4 {
		voiceInstruction = strings.TrimSpace(blocks[2])
	}
	options, _ := parseTaskOptions(strings.TrimSpace(blocks[1]))
	if len(options) > 12 {
		return "TASK_OPTION_LIMIT_EXCEEDED"
	}
	for _, option := range options {
		if !variant.ValidTaskText(option, 500) {
			return "TASK_OPTION_LIMIT_EXCEEDED"
		}
	}
	if !variant.ValidTaskContent(strings.TrimSpace(blocks[0]), options, voiceInstruction, strings.TrimSpace(blocks[len(blocks)-1])) {
		return "TASK_TEXT_LIMIT_EXCEEDED"
	}
	return "TASK_CELL_FRAGMENT"
}
