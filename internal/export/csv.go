// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package export

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
)

// csvSheet writes CSV as a German Excel opens it: UTF-8 with a byte order mark, semicolons,
// CRLF, decimal commas; dates as day.month.year.
type csvSheet struct {
	w     *csv.Writer
	words [2]string
	bom   bool
	out   io.Writer
}

func newCSV(w io.Writer, words [2]string) *csvSheet {
	c := csv.NewWriter(w)
	c.Comma = ';'
	c.UseCRLF = true
	return &csvSheet{w: c, words: words, out: w}
}

func (s *csvSheet) Header(names []string) error {
	if !s.bom {
		s.bom = true
		if _, err := io.WriteString(s.out, "\ufeff"); err != nil {
			return err
		}
	}
	return s.w.Write(names)
}

func (s *csvSheet) Row(cells []Cell) error {
	rec := make([]string, len(cells))
	for i, c := range cells {
		switch c.Kind {
		case Text:
			rec[i] = c.S
		case Number:
			rec[i] = strings.Replace(strconv.FormatFloat(c.F, 'f', -1, 64), ".", ",", 1)
		case Bool:
			rec[i] = s.words[1]
			if c.B {
				rec[i] = s.words[0]
			}
		case Date:
			rec[i] = c.T.Format("02.01.2006")
		case DateTime:
			rec[i] = c.T.Format("02.01.2006 15:04:05")
		}
	}
	return s.w.Write(rec)
}

func (s *csvSheet) Close() error {
	s.w.Flush()
	return s.w.Error()
}
