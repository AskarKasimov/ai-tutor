package csvparser

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

//go:embed testdata/ml_map.csv
var originalMLMap []byte

func TestOriginalMLMapHasTypedMetadataAndKeepsSourceCoordinates(t *testing.T) {
	parsed, err := Parse(originalMLMap)
	if err != nil {
		t.Fatalf("parse original map: %#v", err)
	}
	if parsed.SourceFormat != "ml-map" {
		t.Fatalf("source format = %q, want ml-map", parsed.SourceFormat)
	}
	if got := [4]int{len(parsed.Competencies), len(parsed.Constituents), len(parsed.Outcomes), len(parsed.Tasks)}; got != [4]int{13, 17, 101, 13} {
		t.Fatalf("map counts = competencies/constituents/outcomes/tasks %v, want [13 17 101 13]", got)
	}
	if parsed.UnparsedTaskCells != 41 {
		t.Fatalf("unparsed nonempty task cells = %d, want 41", parsed.UnparsedTaskCells)
	}

	var withContent, included, excluded int
	for _, outcome := range parsed.Outcomes {
		if outcome.EducationalContent != nil {
			withContent++
		}
		if outcome.IncludeInTest == nil || outcome.TaxonomyCode == "" || outcome.ALDLevelCode == "" || outcome.Importance == nil {
			t.Errorf("outcome %q is missing typed ML properties: %#v", outcome.Name, outcome)
			continue
		}
		if *outcome.IncludeInTest {
			included++
		} else {
			excluded++
		}
	}
	if withContent != 18 || included != 18 || excluded != 83 {
		t.Fatalf("typed metadata counts: educational content=%d include_in_test true=%d false=%d; want 18/18/83", withContent, included, excluded)
	}
	for _, constituent := range parsed.Constituents {
		if constituent.TopicLevelCode == "" || len(constituent.Sections) == 0 {
			t.Errorf("constituent %q is missing topic level or RPD section", constituent.Name)
		}
	}

	// The same RPD section has a different set of curriculum competency codes
	// for these two constituents. Keep codes on the constituent-section link.
	var rawPrep, featureSelection []string
	for _, constituent := range parsed.Constituents {
		switch {
		case strings.HasPrefix(constituent.Name, "Умеет подготовить сырые данные"):
			rawPrep = codesForSection(constituent.Sections, "Р.6")
		case strings.HasPrefix(constituent.Name, "Умеет отобрать признаки"):
			featureSelection = codesForSection(constituent.Sections, "Р.6")
		}
	}
	if strings.Join(rawPrep, ",") != "ОПК-2,ОПК-8,ПК-2" {
		t.Errorf("R.6 codes for raw data preparation = %v", rawPrep)
	}
	if strings.Join(featureSelection, ",") != "ОПК-8,ПК-2" {
		t.Errorf("R.6 codes for feature selection = %v", featureSelection)
	}

	first := parsed.Tasks[0]
	if first.Question == "" || first.VoiceInstruction == "" || first.ReferenceAnswer == "" {
		t.Fatalf("first complete task is missing question/instruction/reference answer: %#v", first)
	}
	if len(first.Options) != 4 || first.Options[0] != "классификация" || first.Options[3] != "ранжирование" {
		t.Fatalf("first task options = %v, want the four source options in order", first.Options)
	}
	if !strings.Contains(first.ReferenceAnswer, "целевая переменная") {
		t.Errorf("reference answer was not extracted from Ответ: %q", first.ReferenceAnswer)
	}
	if first.SourceRowIndex < 1 || first.SourceRowIndex > len(parsed.SourceRows) {
		t.Fatalf("task logical row index %d is outside %d source rows", first.SourceRowIndex, len(parsed.SourceRows))
	}
	if first.SourceColumnIndex != 11 {
		t.Fatalf("task source column = %d, want 11 (1-based)", first.SourceColumnIndex)
	}
	row := parsed.SourceRows[first.SourceRowIndex-1]
	if row.Index != first.SourceRowIndex || row.Line != 6 {
		t.Fatalf("task logical/physical source coordinates = %d/%d, want logical %d and physical line 6", row.Index, row.Line, first.SourceRowIndex)
	}
	if !strings.HasPrefix(row.Cells[first.SourceColumnIndex-1], "Экран:") {
		t.Fatal("task logical column does not point back to its original question cell")
	}
	if row.Line == row.Index {
		t.Fatal("fixture should demonstrate that logical record index and physical CSV line differ")
	}
	for i, sourceRow := range parsed.SourceRows {
		if sourceRow.Index != i+1 || sourceRow.Line < 1 {
			t.Errorf("source row[%d] has index/physical line %d/%d", i, sourceRow.Index, sourceRow.Line)
		}
	}
}

func codesForSection(sections []competencymap.CurriculumSection, code string) []string {
	for _, section := range sections {
		if section.Code == code {
			return section.CompetencyCodes
		}
	}
	return nil
}

func TestMLMapRejectsConflictingOutcomeMetadata(t *testing.T) {
	data := ";Компетенция;Уровень темы;Составляющая;Образовательный результат;Что должно войти в тест;Таксономия;Уровень ALDs;Важность;Раздел РПД · компетенции РПД;Задание1;ОС\n" +
		";К;Базовый;С;ОР;TRUE;Знание;базовый;4;\"Р.1 Введение\nОПК-2\";;\n" +
		";К;; ;ОР;FALSE;Знание;базовый;4;\"Р.1 Введение\nОПК-2\";;\n"
	_, err := Parse([]byte(data))
	failure, ok := err.(*fault.Error)
	if !ok || failure.Code != "CSV_INVALID" || len(failure.Details) == 0 {
		t.Fatalf("conflicting duplicate metadata should be a CSV_INVALID error: %#v", err)
	}
	if failure.Details[0].Path != "row:4" || !strings.Contains(failure.Details[0].Message, "Что должно войти в тест") {
		t.Fatalf("conflict should identify the later row and conflicting column: %#v", failure.Details[0])
	}
}
