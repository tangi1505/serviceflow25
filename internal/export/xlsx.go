// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package export

import (
	"io"
	"time"

	"github.com/xuri/excelize/v2"
)

// xlsxSheet writes an Excel workbook with excelize's stream writer (BSD-3-Clause): rows go to
// a temporary file of excelize while they come, the workbook to w on Close. Numbers, truth
// values and dates are typed cells; dates carry a German date format.
type xlsxSheet struct {
	f        *excelize.File
	sw       *excelize.StreamWriter
	w        io.Writer
	row      int
	bold     int
	date     int
	dateTime int
}

const xlsxSheetName = "Export"

func newXLSX(w io.Writer) (*xlsxSheet, error) {
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", xlsxSheetName); err != nil {
		return nil, err
	}
	sw, err := f.NewStreamWriter(xlsxSheetName)
	if err != nil {
		return nil, err
	}
	s := &xlsxSheet{f: f, sw: sw, w: w, row: 1}
	dateFmt, dateTimeFmt := "dd.mm.yyyy", "dd.mm.yyyy hh:mm:ss"
	if s.bold, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err != nil {
		return nil, err
	}
	if s.date, err = f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt}); err != nil {
		return nil, err
	}
	if s.dateTime, err = f.NewStyle(&excelize.Style{CustomNumFmt: &dateTimeFmt}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *xlsxSheet) next(values []any) error {
	cell, err := excelize.CoordinatesToCellName(1, s.row)
	if err != nil {
		return err
	}
	s.row++
	return s.sw.SetRow(cell, values)
}

func (s *xlsxSheet) Header(names []string) error {
	values := make([]any, len(names))
	for i, n := range names {
		values[i] = excelize.Cell{StyleID: s.bold, Value: n}
	}
	return s.next(values)
}

func (s *xlsxSheet) Row(cells []Cell) error {
	values := make([]any, len(cells))
	for i, c := range cells {
		switch c.Kind {
		case Text:
			values[i] = c.S
		case Number:
			values[i] = c.F
		case Bool:
			values[i] = c.B
		case Date:
			values[i] = excelize.Cell{StyleID: s.date, Value: c.T}
		case DateTime:
			// Excel knows no time zones: the moment as the clock in the export's zone shows it.
			t := c.T
			wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
			values[i] = excelize.Cell{StyleID: s.dateTime, Value: wall}
		default:
			values[i] = nil
		}
	}
	return s.next(values)
}

func (s *xlsxSheet) Close() error {
	defer func() { _ = s.f.Close() }()
	if err := s.sw.Flush(); err != nil {
		return err
	}
	_, err := s.f.WriteTo(s.w)
	return err
}
