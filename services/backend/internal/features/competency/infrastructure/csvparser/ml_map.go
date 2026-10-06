package csvparser

import (
	"encoding/csv"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
)

var rpdSection = regexp.MustCompile(`^Р\s*\.\s*(\d+(?:\.\d+)*)\s*(.*)$`)
var rpdCompetency = regexp.MustCompile(`(?:ОПК|ПК)-[0-9]+`)

// parseMLMap accepts the original "Введение в ML" spreadsheet export. Its
// task columns contain both complete assignments and fragments of assignments.
// Every source cell is archived; only self-contained assignments become tasks.
func parseMLMap(reader *csv.Reader, headers []string, headerLine int) (competencymap.Map, error) {
	result := competencymap.Map{SourceFormat: "ml-map", SourceHeaders: append([]string(nil), headers...)}
	columns := make(map[string]int, len(headers))
	taskColumns := []int{}
	allowed := map[string]bool{
		"": true, "Компетенция": true, "Уровень темы": true,
		"Составляющая": true, "Образовательный результат": true,
		"Что должно войти в тест": true, "Таксономия": true,
		"Уровень ALDs": true, "Важность": true,
		"Раздел РПД · компетенции РПД": true, "ОС": true,
	}
	for i, header := range headers {
		if _, exists := columns[header]; exists {
			return result, csvError(headerLine, header, "Повторный заголовок.")
		}
		columns[header] = i
		if header == "" && i != 0 {
			return result, csvError(headerLine, "CSV", "Пустой заголовок допустим только для первого столбца.")
		}
		if match := taskColumn.FindStringSubmatch(header); match != nil {
			if match[1] != "Задание" {
				return result, csvError(headerLine, header, "В этом формате нет парных критериев; требования к ОР находятся в ОС.")
			}
			taskColumns = append(taskColumns, i)
		} else if !allowed[header] {
			return result, csvError(headerLine, header, "Неизвестная колонка карты ML.")
		}
	}
	for _, header := range []string{"Компетенция", "Уровень темы", "Составляющая", "Образовательный результат", "Что должно войти в тест", "Таксономия", "Уровень ALDs", "Важность", "Раздел РПД · компетенции РПД", "ОС"} {
		if _, exists := columns[header]; !exists {
			return result, csvError(headerLine, header, "Обязательная колонка карты отсутствует.")
		}
	}
	if len(taskColumns) == 0 {
		return result, csvError(headerLine, "Задание 1", "Нужна хотя бы одна колонка заданий.")
	}
	seenCompetencies := map[string]bool{}
	seenConstituents := map[string]bool{}
	constituentIndexes := map[string]int{}
	seenOutcomes := map[string]int{}
	competency, constituent := "", ""
	topicLevel := ""
	firstData := true
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, csvReadError(err, headerLine)
		}
		line, _ := reader.FieldPos(0)
		for i := range row {
			row[i] = strings.ReplaceAll(row[i], "\x00", "\r")
			row[i] = strings.ReplaceAll(row[i], xlsxLineBreak, "\n")
		}
		sourceIndex := len(result.SourceRows) + 1
		result.SourceRows = append(result.SourceRows, sourceRow(row, line, sourceIndex))
		if blankRow(row) {
			continue
		}
		if len(row) != len(headers) {
			return result, csvError(line, "CSV", "Число полей ("+strconv.Itoa(len(row))+") не совпадает с заголовками ("+strconv.Itoa(len(headers))+").")
		}
		if index, exists := columns[""]; exists && strings.TrimSpace(row[index]) != "" {
			return result, csvError(line, "CSV", "Безымянный первый столбец должен быть пустым.")
		}
		if firstData && mlExplanationRow(row, columns, taskColumns) {
			firstData = false
			continue
		}
		firstData = false
		outcome := strings.TrimSpace(row[columns["Образовательный результат"]])
		if outcome == "" {
			for _, i := range taskColumns {
				if strings.TrimSpace(row[i]) != "" {
					return result, csvError(line, headers[i], "Задание без образовательного результата.")
				}
			}
			// The final "ПРОВЕРИТЬ: НЕТ В РПД" section contains notes, not
			// outcomes. It remains available in competency_map_source_rows.
			continue
		}
		if value := strings.TrimSpace(row[columns["Компетенция"]]); value != "" {
			if value != competency {
				constituent = ""
				topicLevel = ""
			}
			competency = value
		}
		if value := strings.TrimSpace(row[columns["Составляющая"]]); value != "" {
			if value != constituent {
				topicLevel = ""
			}
			constituent = value
		}
		if value := strings.TrimSpace(row[columns["Уровень темы"]]); value != "" {
			canonical, ok := levelCode(value)
			if !ok {
				return result, csvError(line, "Уровень темы", "Ожидается базовый, средний или продвинутый уровень.")
			}
			if topicLevel != "" && topicLevel != canonical {
				return result, csvError(line, "Уровень темы", "Внутри составляющей указаны разные уровни.")
			}
			topicLevel = canonical
		}
		if competency == "" || constituent == "" {
			return result, csvError(line, "Компетенция/Составляющая", "Для ОР отсутствует родительская запись.")
		}
		cKey := mapKey(competency)
		sKey := mapKey(competency, constituent)
		oKey := mapKey(competency, constituent, outcome)
		if !seenCompetencies[cKey] {
			result.Competencies = append(result.Competencies, competencymap.Competency{Key: cKey, Name: competency})
			seenCompetencies[cKey] = true
		}
		if !seenConstituents[sKey] {
			if topicLevel == "" {
				return result, csvError(line, "Уровень темы", "Для новой составляющей не задан уровень.")
			}
			constituentIndexes[sKey] = len(result.Constituents)
			result.Constituents = append(result.Constituents, competencymap.Constituent{Key: sKey, CompetencyKey: cKey, Name: constituent, TopicLevelCode: topicLevel})
			seenConstituents[sKey] = true
		} else if topicLevel != "" && result.Constituents[constituentIndexes[sKey]].TopicLevelCode != topicLevel {
			return result, csvError(line, "Уровень темы", "Внутри составляющей указаны разные уровни.")
		}
		constituentIndex := constituentIndexes[sKey]
		if rawSection := strings.TrimSpace(row[columns["Раздел РПД · компетенции РПД"]]); rawSection != "" {
			section, ok := parseCurriculumSection(rawSection)
			if !ok {
				return result, csvError(line, "Раздел РПД · компетенции РПД", "Ожидается раздел РПД и список компетенций через точку с запятой.")
			}
			if !mergeCurriculumSection(&result.Constituents[constituentIndex], section) {
				return result, csvError(line, "Раздел РПД · компетенции РПД", "Для одной составляющей раздел указан с разными наборами компетенций.")
			}
		}
		index, exists := seenOutcomes[oKey]
		if !exists {
			index = len(result.Outcomes)
			seenOutcomes[oKey] = index
			result.Outcomes = append(result.Outcomes, competencymap.Outcome{Key: oKey, ConstituentKey: sKey, Name: outcome})
		}
		properties, err := parseOutcomeProperties(row, columns, line)
		if err != nil {
			return result, err
		}
		if err := mergeOutcomeProperties(&result.Outcomes[index], properties, line); err != nil {
			return result, err
		}
		result.Outcomes[index].SourceRowIndexes = append(result.Outcomes[index].SourceRowIndexes, sourceIndex)
		for _, i := range taskColumns {
			if task, ok := parseMLTask(row[i]); ok {
				task.OutcomeKey = oKey
				task.Column = headers[i]
				task.Row = line
				task.SourceRowIndex = sourceIndex
				task.SourceColumnIndex = i + 1
				task.ReferenceAnswer = task.Criteria
				task.Criteria = ""
				result.Tasks = append(result.Tasks, task)
			} else if strings.TrimSpace(row[i]) != "" {
				result.UnparsedTaskCells++
				result.Warnings = append(result.Warnings, competencymap.ImportWarning{Row: line, ColumnIndex: i + 1, Column: headers[i], Code: "TASK_CELL_FRAGMENT"})
			}
		}
	}
	if len(result.Outcomes) == 0 {
		return result, csvError(headerLine+1, "Образовательный результат", "Карта не содержит ОР.")
	}
	return result, nil
}

func levelCode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "базовый":
		return "basic", true
	case "средний":
		return "intermediate", true
	case "продвинутый":
		return "advanced", true
	default:
		return "", false
	}
}

func taxonomyCode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "знание":
		return "knowledge", true
	case "понимание":
		return "understanding", true
	case "применение":
		return "application", true
	case "анализ":
		return "analysis", true
	default:
		return "", false
	}
}

func parseCurriculumSection(value string) (competencymap.CurriculumSection, bool) {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	match := rpdSection.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if match == nil {
		return competencymap.CurriculumSection{}, false
	}
	section := competencymap.CurriculumSection{Code: "Р." + match[1], Title: strings.TrimSpace(match[2]), CompetencyCodes: []string{}}
	for _, line := range lines[1:] {
		for _, field := range strings.FieldsFunc(line, func(r rune) bool { return r == ';' || r == ',' }) {
			code := strings.TrimSpace(field)
			if code == "" {
				continue
			}
			if !rpdCompetency.MatchString(code) || rpdCompetency.FindString(code) != code {
				return competencymap.CurriculumSection{}, false
			}
			section.CompetencyCodes = appendUnique(section.CompetencyCodes, code)
		}
	}
	return section, true
}

func mergeCurriculumSection(constituent *competencymap.Constituent, incoming competencymap.CurriculumSection) bool {
	for i := range constituent.Sections {
		current := &constituent.Sections[i]
		if current.Code != incoming.Code {
			continue
		}
		if current.Title != incoming.Title {
			return false
		}
		if len(current.CompetencyCodes) == 0 {
			current.CompetencyCodes = append(current.CompetencyCodes, incoming.CompetencyCodes...)
			return true
		}
		return len(incoming.CompetencyCodes) == 0 || sameCodes(current.CompetencyCodes, incoming.CompetencyCodes)
	}
	constituent.Sections = append(constituent.Sections, incoming)
	return true
}

func sameCodes(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

type outcomeProperties struct {
	includeInTest      *bool
	taxonomyCode       string
	aldLevelCode       string
	importance         *int
	educationalContent *string
}

func parseOutcomeProperties(row []string, columns map[string]int, line int) (outcomeProperties, error) {
	properties, err := parsePresentOutcomeProperties(row, columns, line)
	if err != nil {
		return properties, err
	}
	if properties.includeInTest == nil || properties.taxonomyCode == "" || properties.aldLevelCode == "" || properties.importance == nil {
		return properties, csvError(line, "Карта", "Для образовательного результата нужны включение в тест, таксономия, ALDs и важность.")
	}
	return properties, nil
}

// Paired maps may contain only some profile columns. Never read an absent column as column zero.
func parsePresentOutcomeProperties(row []string, columns map[string]int, line int) (outcomeProperties, error) {
	var properties outcomeProperties
	cell := func(name string) string {
		if index, ok := columns[name]; ok {
			return strings.TrimSpace(row[index])
		}
		return ""
	}
	if value := cell("Что должно войти в тест"); value != "" {
		var parsed bool
		switch strings.ToUpper(value) {
		case "TRUE":
			parsed = true
		case "FALSE":
			parsed = false
		default:
			return properties, csvError(line, "Что должно войти в тест", "Ожидается TRUE или FALSE.")
		}
		properties.includeInTest = &parsed
	}
	if value := cell("Таксономия"); value != "" {
		code, ok := taxonomyCode(value)
		if !ok {
			return properties, csvError(line, "Таксономия", "Неизвестный уровень таксономии.")
		}
		properties.taxonomyCode = code
	}
	if value := cell("Уровень ALDs"); value != "" {
		code, ok := levelCode(value)
		if !ok {
			return properties, csvError(line, "Уровень ALDs", "Неизвестный уровень ALDs.")
		}
		properties.aldLevelCode = code
	}
	if value := cell("Важность"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 5 {
			return properties, csvError(line, "Важность", "Ожидается целое число от 1 до 5.")
		}
		properties.importance = &parsed
	}
	if value := cell("ОС"); value != "" {
		properties.educationalContent = &value
	}
	return properties, nil
}

func mergeOutcomeProperties(target *competencymap.Outcome, incoming outcomeProperties, line int) error {
	if target.IncludeInTest != nil && incoming.includeInTest != nil && *target.IncludeInTest != *incoming.includeInTest {
		return csvError(line, "Что должно войти в тест", "Для образовательного результата указаны разные значения.")
	}
	if target.TaxonomyCode != "" && incoming.taxonomyCode != "" && target.TaxonomyCode != incoming.taxonomyCode {
		return csvError(line, "Таксономия", "Для образовательного результата указаны разные значения.")
	}
	if target.ALDLevelCode != "" && incoming.aldLevelCode != "" && target.ALDLevelCode != incoming.aldLevelCode {
		return csvError(line, "Уровень ALDs", "Для образовательного результата указаны разные значения.")
	}
	if target.Importance != nil && incoming.importance != nil && *target.Importance != *incoming.importance {
		return csvError(line, "Важность", "Для образовательного результата указаны разные значения.")
	}
	if target.EducationalContent != nil && incoming.educationalContent != nil && *target.EducationalContent != *incoming.educationalContent {
		return csvError(line, "ОС", "Для образовательного результата указаны разные значения.")
	}
	if incoming.includeInTest != nil {
		target.IncludeInTest = incoming.includeInTest
	}
	if incoming.taxonomyCode != "" {
		target.TaxonomyCode = incoming.taxonomyCode
	}
	if incoming.aldLevelCode != "" {
		target.ALDLevelCode = incoming.aldLevelCode
	}
	if incoming.importance != nil {
		target.Importance = incoming.importance
	}
	if incoming.educationalContent != nil {
		target.EducationalContent = incoming.educationalContent
	}
	return nil
}

func csvReadError(err error, headerLine int) error {
	line := headerLine + 1
	var parseError *csv.ParseError
	if errors.As(err, &parseError) {
		line = parseError.Line
	}
	return csvError(line, "CSV", "Некорректное экранирование или структура строки.")
}

func mlExplanationRow(row []string, columns map[string]int, taskColumns []int) bool {
	if strings.TrimSpace(row[columns["Компетенция"]]) != "" ||
		strings.TrimSpace(row[columns["Образовательный результат"]]) != "Сценарий применения на разном уровне" ||
		!strings.HasPrefix(strings.TrimSpace(row[columns["Составляющая"]]), "Что человек должен делать") {
		return false
	}
	for _, i := range taskColumns {
		if strings.TrimSpace(row[i]) != "" {
			return false
		}
	}
	return true
}

func parseMLTask(raw string) (competencymap.Task, bool) {
	var task competencymap.Task
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "Экран:") {
		return task, false
	}
	const instruction = "Голосовая инструкция:"
	const answer = "Ответ:"
	instructionAt := strings.Index(text, instruction)
	answerAt := strings.LastIndex(text, answer)
	if instructionAt < len("Экран:") || answerAt < instructionAt+len(instruction) {
		return task, false
	}
	screen := strings.TrimSpace(text[len("Экран:"):instructionAt])
	voice := strings.TrimSpace(text[instructionAt+len(instruction) : answerAt])
	correct := strings.TrimSpace(text[answerAt+len(answer):])
	if screen == "" || voice == "" || correct == "" {
		return task, false
	}
	if optionsAt := strings.Index(screen, "Варианты:"); optionsAt >= 0 {
		optionText := strings.TrimSpace(screen[optionsAt+len("Варианты:"):])
		screen = strings.TrimSpace(screen[:optionsAt])
		if screen == "" {
			return task, false
		}
		for _, numbered := range strings.Split(optionText, ";") {
			option := strings.TrimSpace(optionNumber.ReplaceAllString(strings.TrimSpace(numbered), ""))
			option = strings.TrimSuffix(option, ".")
			if option == "" {
				return task, false
			}
			task.Options = append(task.Options, option)
		}
	}
	voice = strings.TrimSuffix(voice, ".")
	voice = strings.TrimPrefix(strings.TrimSuffix(voice, "»"), "«")
	task.Question = screen
	task.Criteria = correct
	task.VoiceInstruction = voice
	return task, true
}
