// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package export

import (
	"fmt"
	"io"
	"time"
)

// Kind is what a cell holds; XLSX and ODS keep numbers, truth values and dates as such, CSV
// writes them as text.
type Kind int

const (
	Empty Kind = iota
	Text
	Number
	Bool
	Date     // a day without time
	DateTime // a moment, shown in the time zone of the export
)

// Cell is one value of the table.
type Cell struct {
	Kind Kind
	S    string
	F    float64
	B    bool
	T    time.Time
}

// Formats of the export (UI5; Inventar G230, G231).
const (
	FormatCSV  = "csv"
	FormatXLSX = "xlsx"
	FormatODS  = "ods"
)

// Formats lists the formats in a stable order.
var Formats = []string{FormatCSV, FormatXLSX, FormatODS}

// sheet writes one table: the header once, then the rows, then Close finishes the file.
type sheet interface {
	Header(names []string) error
	Row(cells []Cell) error
	Close() error
}

// newSheet starts a table of that format on w; words are "yes" and "no" in the language of
// the export, for the truth values of a CSV.
func newSheet(format string, w io.Writer, words [2]string) (sheet, error) {
	switch format {
	case FormatCSV:
		return newCSV(w, words), nil
	case FormatXLSX:
		return newXLSX(w)
	case FormatODS:
		return newODS(w)
	}
	return nil, fmt.Errorf("export: unknown format %q", format)
}

// ContentType and Extension name the file of a format.
func ContentType(format string) string {
	switch format {
	case FormatXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case FormatODS:
		return "application/vnd.oasis.opendocument.spreadsheet"
	}
	return "text/csv; charset=utf-8"
}
