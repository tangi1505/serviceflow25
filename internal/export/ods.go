// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package export

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
)

// odsSheet writes an OpenDocument spreadsheet (ODF 1.2) as it goes: the mimetype first and
// uncompressed, as the standard wants, then the manifest, then content.xml row by row. Dates
// carry a date style so that LibreOffice shows them as dates. bufio keeps the first error, so
// the last write of a row and Close report what went wrong in between.
type odsSheet struct {
	z   *zip.Writer
	buf *bufio.Writer
}

const odsMime = "application/vnd.oasis.opendocument.spreadsheet"

const odsManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">
 <manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="application/vnd.oasis.opendocument.spreadsheet"/>
 <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>
`

// The head of content.xml: the date styles of the cells, then the table.
const odsHead = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:number="urn:oasis:names:tc:opendocument:xmlns:datastyle:1.0" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" office:version="1.2">
<office:automatic-styles>
<number:date-style style:name="Nd"><number:day number:style="long"/><number:text>.</number:text><number:month number:style="long"/><number:text>.</number:text><number:year number:style="long"/></number:date-style>
<number:date-style style:name="Ndt"><number:day number:style="long"/><number:text>.</number:text><number:month number:style="long"/><number:text>.</number:text><number:year number:style="long"/><number:text> </number:text><number:hours number:style="long"/><number:text>:</number:text><number:minutes number:style="long"/><number:text>:</number:text><number:seconds number:style="long"/></number:date-style>
<style:style style:name="cd" style:family="table-cell" style:data-style-name="Nd"/>
<style:style style:name="cdt" style:family="table-cell" style:data-style-name="Ndt"/>
<style:style style:name="ch" style:family="table-cell"><style:text-properties fo:font-weight="bold"/></style:style>
</office:automatic-styles>
<office:body><office:spreadsheet><table:table table:name="Export">
`

const odsTail = "</table:table></office:spreadsheet></office:body></office:document-content>\n"

func newODS(w io.Writer) (*odsSheet, error) {
	z := zip.NewWriter(w)
	// Raw with its size and checksum: no data descriptor, so that readers that look at the
	// first bytes of the file find the type there.
	m, err := z.CreateRaw(&zip.FileHeader{Name: "mimetype", Method: zip.Store, CRC32: crc32.ChecksumIEEE([]byte(odsMime)),
		CompressedSize64: uint64(len(odsMime)), UncompressedSize64: uint64(len(odsMime))})
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(m, odsMime); err != nil {
		return nil, err
	}
	mf, err := z.Create("META-INF/manifest.xml")
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(mf, odsManifest); err != nil {
		return nil, err
	}
	c, err := z.Create("content.xml")
	if err != nil {
		return nil, err
	}
	buf := bufio.NewWriterSize(c, 64<<10)
	if _, err := buf.WriteString(odsHead); err != nil {
		return nil, err
	}
	return &odsSheet{z: z, buf: buf}, nil
}

// text escapes a value for the XML of a cell.
func (s *odsSheet) text(v string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}

func (s *odsSheet) Header(names []string) error {
	_, _ = s.buf.WriteString("<table:table-row>")
	for _, n := range names {
		fmt.Fprintf(s.buf, `<table:table-cell table:style-name="ch" office:value-type="string"><text:p>%s</text:p></table:table-cell>`, s.text(n))
	}
	_, err := s.buf.WriteString("</table:table-row>\n")
	return err
}

func (s *odsSheet) Row(cells []Cell) error {
	_, _ = s.buf.WriteString("<table:table-row>")
	for _, c := range cells {
		switch c.Kind {
		case Text:
			fmt.Fprintf(s.buf, `<table:table-cell office:value-type="string"><text:p>%s</text:p></table:table-cell>`, s.text(c.S))
		case Number:
			v := strconv.FormatFloat(c.F, 'f', -1, 64)
			fmt.Fprintf(s.buf, `<table:table-cell office:value-type="float" office:value="%s"><text:p>%s</text:p></table:table-cell>`, v, v)
		case Bool:
			fmt.Fprintf(s.buf, `<table:table-cell office:value-type="boolean" office:boolean-value="%t"><text:p>%t</text:p></table:table-cell>`, c.B, c.B)
		case Date:
			fmt.Fprintf(s.buf, `<table:table-cell table:style-name="cd" office:value-type="date" office:date-value="%s"><text:p>%s</text:p></table:table-cell>`,
				c.T.Format("2006-01-02"), c.T.Format("02.01.2006"))
		case DateTime:
			fmt.Fprintf(s.buf, `<table:table-cell table:style-name="cdt" office:value-type="date" office:date-value="%s"><text:p>%s</text:p></table:table-cell>`,
				c.T.Format("2006-01-02T15:04:05"), c.T.Format("02.01.2006 15:04:05"))
		default:
			_, _ = s.buf.WriteString("<table:table-cell/>")
		}
	}
	_, err := s.buf.WriteString("</table:table-row>\n")
	return err
}

func (s *odsSheet) Close() error {
	if _, err := s.buf.WriteString(odsTail); err != nil {
		return err
	}
	if err := s.buf.Flush(); err != nil {
		return err
	}
	return s.z.Close()
}
