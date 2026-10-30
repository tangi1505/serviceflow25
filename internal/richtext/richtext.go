// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package richtext keeps formatted text safe (UI7; Hauptplan E127). The editor in the browser
// (TipTap) sends HTML; the server lets through a fixed list of elements and attributes with
// bluemonday (BSD-3-Clause) and nothing else, so that no script, style or foreign address
// reaches another reader: images only from the own file store, links only to http, https
// and mailto. Text gives the plain text for mails, lists and exports.
package richtext

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// imageSrc is the address of an image in the own file store.
var imageSrc = regexp.MustCompile(`^/api/v1/files/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

var policy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "b", "em", "i", "u", "s", "ul", "ol", "li",
		"h2", "h3", "h4", "blockquote", "code", "pre", "hr",
		"table", "thead", "tbody", "tr", "th", "td")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	// Images point into the own store by a path without host.
	p.AllowRelativeURLs(true)
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("src").Matching(imageSrc).OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowAttrs("colspan", "rowspan").Matching(regexp.MustCompile(`^[1-9][0-9]?$`)).OnElements("td", "th")
	// An image without an allowed source is no image.
	p.AllowNoAttrs().OnElements("img")
	return p
}()

// Sanitize returns the HTML with only the allowed elements and attributes.
func Sanitize(h string) string {
	out := policy.Sanitize(h)
	// An img that lost its source is dropped: it would show a broken picture.
	return emptyImg.ReplaceAllString(out, "")
}

var emptyImg = regexp.MustCompile(`<img(?:\s+alt="[^"]*")?\s*/?>`)

// Images lists the files the HTML shows, each once, in order.
func Images(h string) []uuid.UUID {
	var out []uuid.UUID
	seen := map[uuid.UUID]bool{}
	z := html.NewTokenizer(strings.NewReader(h))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		t := z.Token()
		if t.Data != "img" {
			continue
		}
		for _, a := range t.Attr {
			if a.Key != "src" {
				continue
			}
			if m := imageSrc.FindStringSubmatch(a.Val); m != nil {
				if id, err := uuid.Parse(m[1]); err == nil && !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
		}
	}
}

// blocks end a line in the plain text.
var blocks = map[string]bool{"p": true, "br": true, "li": true, "h2": true, "h3": true, "h4": true,
	"blockquote": true, "pre": true, "tr": true, "hr": true, "ul": true, "ol": true, "table": true}

// Text returns the plain text of the HTML: blocks and line breaks as new lines, list items
// with a dash, links with their address, images as [Bild].
func Text(h string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(h))
	var href string
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(collapse(b.String()))
		case html.TextToken:
			b.WriteString(string(z.Text()))
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			switch t.Data {
			case "li":
				b.WriteString("\n- ")
			case "br":
				b.WriteString("\n")
			case "img":
				b.WriteString("[Bild]")
			case "a":
				href = ""
				for _, a := range t.Attr {
					if a.Key == "href" {
						href = a.Val
					}
				}
			case "td", "th":
				b.WriteString("\t")
			}
		case html.EndTagToken:
			t := z.Token()
			if t.Data == "a" && href != "" {
				b.WriteString(" (" + href + ")")
				href = ""
			}
			if blocks[t.Data] && t.Data != "li" {
				b.WriteString("\n")
			}
		}
	}
}

var manyBreaks = regexp.MustCompile(`\n{3,}`)

func collapse(s string) string { return manyBreaks.ReplaceAllString(s, "\n\n") }
