// Package xlsx reads the worksheets of an Excel book as rows of cell text.
// It needs only the standard library: a competency map is plain text cells.
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/entities/coursegraph"
)

// Limits guard against a crafted book that unpacks into far more than it weighs.
const (
	maxPartBytes = 64 << 20
	maxRows      = 20000
	maxColumns   = 200
)

// ErrNotWorkbook means the file is not an .xlsx book.
var ErrNotWorkbook = errors.New("not an xlsx workbook")

type reader struct {
	files map[string]*zip.File
}

// Read returns the worksheets in book order.
func Read(data []byte) ([]coursegraph.MapSheet, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrNotWorkbook
	}
	r := reader{files: map[string]*zip.File{}}
	for _, f := range archive.File {
		r.files[f.Name] = f
	}
	var book struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := r.decode("xl/workbook.xml", &book); err != nil {
		return nil, ErrNotWorkbook
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := r.decode("xl/_rels/workbook.xml.rels", &rels); err != nil {
		return nil, ErrNotWorkbook
	}
	targets := map[string]string{}
	for _, rel := range rels.Items {
		target := strings.TrimPrefix(rel.Target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Join("xl", target)
		}
		targets[rel.ID] = target
	}
	shared, err := r.sharedStrings()
	if err != nil {
		return nil, err
	}
	sheets := make([]coursegraph.MapSheet, 0, len(book.Sheets))
	for _, s := range book.Sheets {
		target, ok := targets[s.ID]
		if !ok {
			continue
		}
		rows, err := r.rows(target, shared)
		if err != nil {
			return nil, fmt.Errorf("sheet %q: %w", s.Name, err)
		}
		sheets = append(sheets, coursegraph.MapSheet{Name: s.Name, Rows: rows})
	}
	return sheets, nil
}

func (r reader) open(name string) (io.ReadCloser, error) {
	f, ok := r.files[name]
	if !ok {
		return nil, fmt.Errorf("%s is missing", name)
	}
	if f.UncompressedSize64 > maxPartBytes {
		return nil, fmt.Errorf("%s is too large", name)
	}
	return f.Open()
}

func (r reader) decode(name string, v any) error {
	rc, err := r.open(name)
	if err != nil {
		return err
	}
	defer rc.Close()
	return xml.NewDecoder(io.LimitReader(rc, maxPartBytes)).Decode(v)
}

// text joins the runs of a string item, rich text included.
type text struct {
	Plain string `xml:"t"`
	Runs  []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

func (t text) String() string {
	if len(t.Runs) == 0 {
		return t.Plain
	}
	var b strings.Builder
	for _, run := range t.Runs {
		b.WriteString(run.Text)
	}
	return b.String()
}

func (r reader) sharedStrings() ([]string, error) {
	if _, ok := r.files["xl/sharedStrings.xml"]; !ok {
		return nil, nil
	}
	var table struct {
		Items []text `xml:"si"`
	}
	if err := r.decode("xl/sharedStrings.xml", &table); err != nil {
		return nil, fmt.Errorf("shared strings: %w", err)
	}
	out := make([]string, len(table.Items))
	for i, item := range table.Items {
		out[i] = item.String()
	}
	return out, nil
}

func (r reader) rows(name string, shared []string) ([][]string, error) {
	var sheet struct {
		Rows []struct {
			Number int `xml:"r,attr"`
			Cells  []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline text   `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := r.decode(name, &sheet); err != nil {
		return nil, err
	}
	rows := [][]string{}
	for _, row := range sheet.Rows {
		if row.Number < 1 || row.Number > maxRows {
			return nil, fmt.Errorf("row %d is out of range", row.Number)
		}
		// Rows are numbered in the file; missing ones are empty.
		for len(rows) < row.Number {
			rows = append(rows, nil)
		}
		cells := []string{}
		for i, c := range row.Cells {
			column := i
			if c.Ref != "" {
				var err error
				if column, err = columnIndex(c.Ref); err != nil {
					return nil, err
				}
			}
			if column >= maxColumns {
				return nil, fmt.Errorf("column %d is out of range", column+1)
			}
			for len(cells) <= column {
				cells = append(cells, "")
			}
			switch c.Type {
			case "s":
				var index int
				if _, err := fmt.Sscan(c.Value, &index); err != nil || index < 0 || index >= len(shared) {
					return nil, fmt.Errorf("cell %s refers to an unknown string", c.Ref)
				}
				cells[column] = shared[index]
			case "inlineStr":
				cells[column] = c.Inline.String()
			case "b":
				cells[column] = map[string]string{"1": "TRUE", "0": "FALSE"}[c.Value]
			default:
				cells[column] = c.Value
			}
		}
		rows[row.Number-1] = cells
	}
	return rows, nil
}

// columnIndex reads the column of a cell reference such as "AB12".
func columnIndex(ref string) (int, error) {
	n := 0
	for _, ch := range ref {
		if ch < 'A' || ch > 'Z' {
			break
		}
		n = n*26 + int(ch-'A'+1)
	}
	if n == 0 {
		return 0, fmt.Errorf("bad cell reference %q", ref)
	}
	return n - 1, nil
}
