package xlsx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func book(t *testing.T, parts map[string]string) []byte {
	t.Helper()
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

const ns = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

func TestReadReturnsTheCellsOfEverySheet(t *testing.T) {
	data := book(t, map[string]string{
		"xl/workbook.xml": `<workbook ` + ns + `><sheets><sheet name="Карта" sheetId="1" r:id="rId1"/><sheet name="Легенда" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="/xl/worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml": `<sst ` + ns + `><si><t>Субкомпетенция</t></si><si><r><t>Назовите </t></r><r><t>агента.</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet ` + ns + `><sheetData>` +
			`<row r="1"><c r="B1" t="s"><v>0</v></c></row>` +
			`<row r="3"><c r="A3"><v>4.0</v></c><c r="C3" t="s"><v>1</v></c><c r="D3" t="b"><v>1</v></c><c r="E3" t="inlineStr"><is><t>текст</t></is></c></row>` +
			`</sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet ` + ns + `><sheetData/></worksheet>`,
	})
	sheets, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 || sheets[0].Name != "Карта" || sheets[1].Name != "Легенда" || len(sheets[1].Rows) != 0 {
		t.Fatalf("sheets = %+v", sheets)
	}
	rows := sheets[0].Rows
	if len(rows) != 3 || rows[0][1] != "Субкомпетенция" || rows[1] != nil {
		t.Fatalf("rows = %q", rows)
	}
	if got := rows[2]; len(got) != 5 || got[0] != "4.0" || got[1] != "" || got[2] != "Назовите агента." || got[3] != "TRUE" || got[4] != "текст" {
		t.Fatalf("row 3 = %q", got)
	}
}

func TestReadRejectsWhatIsNotABook(t *testing.T) {
	if _, err := Read([]byte("%PDF-1.7")); !errors.Is(err, ErrNotWorkbook) {
		t.Fatalf("pdf: %v", err)
	}
	if _, err := Read(book(t, map[string]string{"word/document.xml": "<document/>"})); !errors.Is(err, ErrNotWorkbook) {
		t.Fatalf("docx: %v", err)
	}
}

func TestReadRefusesALongCellReference(t *testing.T) {
	for _, ref := range []string{"ZZZZZZZZZZZZZZ1", "HZ1", "A", "1A", "A1B"} {
		data := book(t, map[string]string{
			"xl/workbook.xml":            `<workbook ` + ns + `><sheets><sheet name="Карта" sheetId="1" r:id="rId1"/></sheets></workbook>`,
			"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
			"xl/worksheets/sheet1.xml":   `<worksheet ` + ns + `><sheetData><row r="1"><c r="` + ref + `"><v>1</v></c></row></sheetData></worksheet>`,
		})
		if _, err := Read(data); err == nil {
			t.Fatalf("cell %s accepted", ref)
		}
	}
}

func TestReadRefusesABookThatMultipliesItsParts(t *testing.T) {
	var sheets, rels strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&sheets, `<sheet name="Лист %d" sheetId="%d" r:id="rId%d"/>`, i, i, i)
		// Every entry points at the same part.
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Target="worksheets/sheet1.xml"/>`, i)
	}
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook ` + ns + `><sheets>` + sheets.String() + `</sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels.String() + `</Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet ` + ns + `><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + strings.Repeat("x", 1<<20) + `</t></is></c></row></sheetData></worksheet>`,
	}
	if _, err := Read(book(t, parts)); err == nil || !strings.Contains(err.Error(), "repeats part") {
		t.Fatalf("a part read through many sheets: %v", err)
	}
	sheets.Reset()
	for i := 1; i <= maxSheets+1; i++ {
		fmt.Fprintf(&sheets, `<sheet name="Лист %d" sheetId="%d" r:id="rId1"/>`, i, i)
	}
	parts["xl/workbook.xml"] = `<workbook ` + ns + `><sheets>` + sheets.String() + `</sheets></workbook>`
	if _, err := Read(book(t, parts)); err == nil || !strings.Contains(err.Error(), "sheets") {
		t.Fatalf("too many sheets: %v", err)
	}
}

func TestReadSpendsOneBudgetForTheWholeBook(t *testing.T) {
	r := &reader{cells: 2, text: 6}
	if r.spend("абв") != nil || r.spend("") != nil {
		t.Fatal("cells within the budget refused")
	}
	if r.spend("") == nil {
		t.Fatal("a cell past the budget accepted")
	}
	r = &reader{cells: 10, text: 3}
	if r.spend("abcd") == nil {
		t.Fatal("text past the budget accepted")
	}
}
