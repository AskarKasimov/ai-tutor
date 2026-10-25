package csvparser

import (
	"errors"
	"strings"
	"testing"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
	"github.com/xuri/excelize/v2"
)

func TestPairedMapRejectsEquivalentTaskHeaders(t *testing.T) {
	for _, extra := range []string{"Критерии1", "Задание1", "Критерии 01", "Задание 01"} {
		t.Run(extra, func(t *testing.T) {
			_, err := Parse([]byte("Ком;Сост;ОР;Задание 1;Критерии 1;" + extra + "\nК;С;О;Вопрос;Критерии;Перезапись\n"))
			var failure *fault.Error
			if !errors.As(err, &failure) || failure.Code != "CSV_INVALID" || len(failure.Details) != 1 || failure.Details[0].Path != "row:1" || !strings.Contains(failure.Details[0].Message, extra) {
				t.Fatalf("duplicate %q must identify header: %#v", extra, err)
			}
		})
	}
}

func TestMLMapRejectsConflictingSectionTitlesAcrossConstituents(t *testing.T) {
	data := "Компетенция;Составляющая;Образовательный результат;Уровень темы;Что должно войти в тест;Таксономия;Уровень ALDs;Важность;Раздел РПД · компетенции РПД;ОС;Задание1\nК;С1;О1;Базовый;TRUE;Знание;Базовый;3;Р.1 Введение;;\n;С2;О2;Базовый;TRUE;Знание;Базовый;3;Р.1 Другая тема;;\n"
	_, err := Parse([]byte(data))
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Code != "CSV_INVALID" || len(failure.Details) != 1 || failure.Details[0].Path != "row:3" || !strings.Contains(failure.Details[0].Message, "Раздел РПД · компетенции РПД") {
		t.Fatalf("conflicting section must identify source cell: %#v", err)
	}
}

func TestXLSXPreservesCriteriaAndSourceText(t *testing.T) {
	text := "Первый\r\nВторой\rТретий\nЧетвёртый \\n \\r \\ \ue000"
	book := excelize.NewFile()
	defer book.Close()
	sheet := book.GetSheetName(0)
	for row, cells := range [][]string{{"Ком", "Сост", "ОР", "Задание 1", "Критерии 1"}, {"К", "С", "О", "Вопрос", text}} {
		for column, value := range cells {
			cell, _ := excelize.CoordinatesToCellName(column+1, row+1)
			if err := book.SetCellValue(sheet, cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tasks) != 1 || parsed.Tasks[0].Criteria != text || parsed.SourceRows[0].Cells[4] != text || parsed.Tasks[0].Row != 2 {
		t.Fatalf("criteria/source must preserve text and worksheet row: %#v / %#v", parsed.Tasks, parsed.SourceRows)
	}
}
