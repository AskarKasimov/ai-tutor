package app

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

// mapBook is a competency map book of one sheet in the course template:
// one competency with three outcomes for the test, so its graph has a main
// task and two basic ones.
func mapBook(t *testing.T) []byte {
	t.Helper()
	rows := [][]string{
		{"№", "Субкомпетенция (Proficiency)", "Составляющая (Observe)", "Образовательный результат (Observe Behaviour)",
			"Уровень по Блуму", "Важность (1–5)", "Что должно войти в тест", "Задание (устная инструкция)", "Критерии оценивания"},
		{"1", "К1. Агент", "1.1 Описывает агента", "Называет части агента", "Помнить", "4", "TRUE", "Назовите части агента.", "Эталон: сенсоры и исполнительные механизмы."},
		{"2", "", "", "Объясняет рациональность", "Понимать", "3", "TRUE", "Объясните, что такое рациональный агент.", "Эталон: максимизирует меру эффективности."},
		{"3", "", "", "Выбирает тип агента", "Анализировать", "5", "TRUE", "Какой агент нужен роботу-уборщику и почему?", "Эталон: агент с моделью мира.\nБалл (макс. 1): 1 — тип и причина."},
	}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, value := range row {
			var text bytes.Buffer
			if err := xml.EscapeText(&text, []byte(value)); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t>%s</t></is></c>`, 'A'+j, i+1, text.String())
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	parts := map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Карта компетенций" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": sheet.String(),
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
