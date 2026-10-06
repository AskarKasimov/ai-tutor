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
const xlsxLineBreak = "\ue000"

func parseXLSX(data []byte) (competencymap.Map, error) {
	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		RawCellValue: true, UnzipSizeLimit: 128 << 20, UnzipXMLSizeLimit: 16 << 20,
	})
	if err != nil {
		return competencymap.Map{}, csvError(1, "XLSX", "Не удалось прочитать книгу Excel.")
	}
	defer workbook.Close()

	var selected [][]string
	var selectedHeader int
	for _, sheet := range workbook.GetSheetList() {
		rows, err := worksheetRows(workbook, sheet)
		if err != nil {
			return competencymap.Map{}, csvError(1, "XLSX", "Размер листа превышает предел или лист повреждён.")
		}
		header := headerIndex(rows)
		if header < 0 {
			continue
		}
		if selected != nil {
			return competencymap.Map{}, csvError(1, "XLSX", "Книга должна содержать ровно один лист с картой компетенций.")
		}
		selected = rows
		selectedHeader = header
	}
	if selected == nil {
		return competencymap.Map{}, csvError(1, "XLSX", "Не найден лист с заголовками карты компетенций.")
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
	return Parse(encoded.Bytes())
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
			rows[i][j] = strings.ReplaceAll(rows[i][j], "\n", xlsxLineBreak)
			rows[i][j] = strings.ReplaceAll(rows[i][j], "\r", xlsxLineBreak)
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
			found[value] = true
		}
		if found["Ком"] && found["Сост"] && found["ОР"] ||
			found["Компетенция"] && found["Составляющая"] && found["Образовательный результат"] {
			return i
		}
	}
	return -1
}
