package csvparser

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXMapPreservesBlankRowsAndParsesSourceCoordinates(t *testing.T) {
	workbook := excelize.NewFile()
	sheet := workbook.GetSheetName(0)
	if err := workbook.SetCellValue(sheet, "A1", "Заметки"); err != nil {
		t.Fatal(err)
	}

	reader := csv.NewReader(bytes.NewReader(originalMLMap))
	reader.FieldsPerRecord = -1
	for rowIndex := 1; ; rowIndex++ {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if rowIndex == 1 && len(row) > 0 {
			row[0] = strings.TrimPrefix(row[0], "\ufeff")
		}
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				t.Fatal(err)
			}
			if value != "" {
				if err := workbook.SetCellValue(sheet, cell, value); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := workbook.SetCellValue(sheet, "A2", ""); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue(sheet, "N2", ""); err != nil {
		t.Fatal(err)
	}
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(buffer.Bytes())
	if err != nil {
		t.Fatalf("parse generated XLSX: %#v", err)
	}
	if len(parsed.Outcomes) != 101 || len(parsed.Tasks) != 13 || parsed.UnparsedTaskCells != 41 {
		t.Fatalf("parsed XLSX counts: outcomes=%d tasks=%d fragments=%d", len(parsed.Outcomes), len(parsed.Tasks), parsed.UnparsedTaskCells)
	}
	firstTask := parsed.Tasks[0]
	if firstTask.SourceColumnIndex != 11 || firstTask.SourceRowIndex <= 0 {
		t.Fatalf("source coordinate: row=%d column=%d", firstTask.SourceRowIndex, firstTask.SourceColumnIndex)
	}
	if !strings.HasPrefix(parsed.SourceRows[firstTask.SourceRowIndex-1].Cells[firstTask.SourceColumnIndex-1], "Экран:") {
		t.Fatal("XLSX source coordinates do not point to original task cell")
	}
}

func TestXLSXRejectsMultipleMapSheets(t *testing.T) {
	workbook := excelize.NewFile()
	first := workbook.GetSheetName(0)
	for _, sheet := range []string{first, "Second"} {
		if sheet == "Second" {
			if _, err := workbook.NewSheet(sheet); err != nil {
				t.Fatal(err)
			}
		}
		for column, value := range []string{"Ком", "Сост", "ОР", "Задание 1", "Критерии 1"} {
			cell, err := excelize.CoordinatesToCellName(column+1, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := workbook.SetCellValue(sheet, cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(buffer.Bytes()); err == nil || !strings.Contains(err.Error(), "CSV_INVALID") {
		t.Fatalf("multiple map sheets should be rejected, got %v", err)
	}
}

func TestXLSXNativeBooleanValues(t *testing.T) {
	for _, value := range []bool{true, false} {
		workbook := excelize.NewFile()
		sheet := workbook.GetSheetName(0)
		headers := []string{"Компетенция", "Составляющая", "Образовательный результат", "Уровень темы", "Что должно войти в тест", "Таксономия", "Уровень ALDs", "Важность", "Раздел РПД · компетенции РПД", "ОС", "Задание1"}
		row := []any{"К", "С", "О", "Базовый", value, "Знание", "Базовый", 3, "", "", ""}
		for i, header := range headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			if err := workbook.SetCellValue(sheet, cell, header); err != nil {
				t.Fatal(err)
			}
			cell, _ = excelize.CoordinatesToCellName(i+1, 2)
			if err := workbook.SetCellValue(sheet, cell, row[i]); err != nil {
				t.Fatal(err)
			}
		}
		buffer, err := workbook.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		workbook.Close()
		parsed, err := Parse(buffer.Bytes())
		if err != nil {
			t.Fatalf("boolean %v: %v", value, err)
		}
		if len(parsed.Outcomes) != 1 || parsed.Outcomes[0].IncludeInTest == nil || *parsed.Outcomes[0].IncludeInTest != value {
			t.Fatalf("boolean %v: %#v", value, parsed.Outcomes)
		}
	}
}
