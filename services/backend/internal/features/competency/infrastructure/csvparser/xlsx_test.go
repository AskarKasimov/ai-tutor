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
	if err := workbook.SetCellValue(sheet, "A1", "Заметки"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue(sheet, "A2", ""); err != nil {
		t.Fatal(err)
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
