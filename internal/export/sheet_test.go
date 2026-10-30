// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package export

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

var sampleRow = []Cell{
	{Kind: Text, S: `Drucker "3. Stock"; <defekt> & mehr`},
	{Kind: Number, F: 1234.5},
	{Kind: Bool, B: true},
	{Kind: Date, T: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)},
	{Kind: DateTime, T: time.Date(2026, 10, 29, 14, 5, 9, 0, time.UTC)},
	{},
}

func write(t *testing.T, format string) []byte {
	t.Helper()
	var buf bytes.Buffer
	sh, err := newSheet(format, &buf, [2]string{"ja", "nein"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.Header([]string{"Titel", "Betrag", "Aktiv", "Fällig", "Gesehen", "Leer"}); err != nil {
		t.Fatal(err)
	}
	if err := sh.Row(sampleRow); err != nil {
		t.Fatal(err)
	}
	if err := sh.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// CSV as a German Excel opens it: BOM, semicolons, quotes doubled, decimal comma, CRLF.
func TestCSV(t *testing.T) {
	got := string(write(t, FormatCSV))
	want := "\ufeffTitel;Betrag;Aktiv;Fällig;Gesehen;Leer\r\n" +
		`"Drucker ""3. Stock""; <defekt> & mehr";1234,5;ja;29.10.2026;29.10.2026 14:05:09;` + "\r\n"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func entries(t *testing.T, b []byte) (*zip.Reader, map[string]string) {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(r)
		_ = r.Close()
		out[f.Name] = string(data)
	}
	return z, out
}

// ODS: mimetype first and stored, so that the type is in the first bytes; typed cells with the
// text escaped and the dates styled.
func TestODS(t *testing.T) {
	b := write(t, FormatODS)
	if !bytes.Contains(b[:100], []byte("mimetypeapplication/vnd.oasis.opendocument.spreadsheet")) {
		t.Fatalf("mimetype is not first and stored: %q", b[:100])
	}
	z, files := entries(t, b)
	if z.File[0].Name != "mimetype" || z.File[0].Method != zip.Store {
		t.Fatalf("first entry: %+v", z.File[0].FileHeader)
	}
	c := files["content.xml"]
	for _, want := range []string{
		`<text:p>Drucker &#34;3. Stock&#34;; &lt;defekt&gt; &amp; mehr</text:p>`,
		`office:value-type="float" office:value="1234.5"`,
		`office:value-type="boolean" office:boolean-value="true"`,
		`table:style-name="cd" office:value-type="date" office:date-value="2026-10-29"`,
		`office:date-value="2026-10-29T14:05:09"`,
		`<table:table-cell/>`,
		`table:style-name="ch" office:value-type="string"><text:p>Fällig</text:p>`,
	} {
		if !strings.Contains(c, want) {
			t.Errorf("content.xml lacks %s", want)
		}
	}
	if !strings.Contains(files["META-INF/manifest.xml"], `manifest:full-path="content.xml"`) {
		t.Error("manifest lacks content.xml")
	}
}

// XLSX: typed cells a reader gets back, the header bold, dates as dates.
func TestXLSX(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(write(t, FormatXLSX)))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(xlsxSheetName, excelize.Options{RawCellValue: true})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows: %v %v", rows, err)
	}
	if rows[0][3] != "Fällig" || rows[1][0] != `Drucker "3. Stock"; <defekt> & mehr` || rows[1][1] != "1234.5" || rows[1][2] != "1" {
		t.Fatalf("values: %q", rows)
	}
	for cell, want := range map[string]excelize.CellType{"B2": excelize.CellTypeNumber, "C2": excelize.CellTypeBool, "D2": excelize.CellTypeNumber} {
		if got, _ := f.GetCellType(xlsxSheetName, cell); got != want && (want != excelize.CellTypeNumber || got != excelize.CellTypeUnset) {
			t.Errorf("%s: type %v, want %v", cell, got, want)
		}
	}
	shown, _ := f.GetCellValue(xlsxSheetName, "E2")
	if shown != "29.10.2026 14:05:09" {
		t.Errorf("date and time shown as %q", shown)
	}
	if day, _ := f.GetCellValue(xlsxSheetName, "D2"); day != "29.10.2026" {
		t.Errorf("date shown as %q", day)
	}
}

func TestFileName(t *testing.T) {
	if got := FileName("probe.probe", FormatXLSX, time.Date(2026, 10, 29, 9, 7, 0, 0, time.UTC)); got != "probe-probe-2026-10-29-0907.xlsx" {
		t.Fatal(got)
	}
}
