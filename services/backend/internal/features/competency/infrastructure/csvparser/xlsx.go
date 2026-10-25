package csvparser

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/competencymap"
)

const maxWorksheetRows = 10000
const maxWorksheetColumns = 100

// Escape cell line breaks before the CSV bridge so worksheet row numbers stay physical.
var escapeXLSXCell = strings.NewReplacer("\\", "\\\\", "\r", "\\r", "\n", "\\n")
var restoreXLSXCell = strings.NewReplacer("\\\\", "\\", "\\r", "\r", "\\n", "\n")

func parseXLSX(data []byte) (competencymap.Map, error) {
	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		RawCellValue: true, UnzipSizeLimit: 128 << 20, UnzipXMLSizeLimit: 16 << 20,
	})
	if err != nil {
		return competencymap.Map{}, csvError(1, "XLSX", "Не удалось прочитать книгу Excel.")
	}
	defer workbook.Close()

	// One book serves both modes of the stand: the exam simulation reads its
	// first sheet and the diagnostic its second; a book of one sheet is read
	// as before.
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return competencymap.Map{}, csvError(1, "XLSX", "В книге нет листов.")
	}
	sheet := sheets[0]
	if len(sheets) > 1 {
		sheet = sheets[1]
	}
	selected, err := worksheetRows(workbook, sheet)
	if err != nil {
		return competencymap.Map{}, csvError(1, "XLSX", "Размер листа превышает предел или лист повреждён.")
	}
	selectedHeader := headerIndex(selected)
	if selectedHeader < 0 {
		message := "Не найдены заголовки карты компетенций на листе «" + sheet + "»."
		if len(sheets) > 1 {
			message = "Не найдены заголовки карты компетенций на втором листе «" + sheet + "»: диагностика читает второй лист книги, первый — для симуляции экзамена."
		}
		return competencymap.Map{}, csvError(1, "XLSX", message)
	}

	var encoded bytes.Buffer
	writer := csv.NewWriter(&encoded)
	for range selected[:selectedHeader] {
		if err := writer.Write(make([]string, len(selected[selectedHeader]))); err != nil {
			return competencymap.Map{}, err
		}
	}
	for _, row := range selected[selectedHeader:] {
		if err := writer.Write(row); err != nil {
			return competencymap.Map{}, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return competencymap.Map{}, err
	}
	return parseCSV(encoded.Bytes(), restoreXLSXCell.Replace)
}

func worksheetRows(workbook *excelize.File, sheet string) ([][]string, error) {
	rows, err := workbook.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, err
	}
	if len(rows) > maxWorksheetRows {
		return nil, io.ErrShortBuffer
	}
	lastColumn := 0
	for _, row := range rows {
		lastColumn = max(lastColumn, len(row))
	}
	if lastColumn > maxWorksheetColumns {
		return nil, io.ErrShortBuffer
	}
	for i := range rows {
		if len(rows[i]) < lastColumn {
			rows[i] = append(rows[i], make([]string, lastColumn-len(rows[i]))...)
		}
		for j := range rows[i] {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				return nil, err
			}
			kind, err := workbook.GetCellType(sheet, cell)
			if err != nil {
				return nil, err
			}
			if kind == excelize.CellTypeBool {
				switch rows[i][j] {
				case "1":
					rows[i][j] = "TRUE"
				case "0":
					rows[i][j] = "FALSE"
				}
			}
			rows[i][j] = escapeXLSXCell.Replace(rows[i][j])
		}
	}
	return rows, nil
}

func headerIndex(rows [][]string) int {
	for i, row := range rows {
		if blankRow(row) {
			continue
		}
		found := make(map[string]bool, len(row))
		for _, value := range row {
			found[strings.TrimSpace(value)] = true
		}
		if found["Ком"] && found["Сост"] && found["ОР"] ||
			found["Компетенция"] && found["Составляющая"] && found["Образовательный результат"] {
			return i
		}
	}
	return -1
}
