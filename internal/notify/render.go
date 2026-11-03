// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package notify

import (
	"fmt"
	"regexp"
	"strings"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/richtext"
)

// Placeholders without logic (WF15): {{record.<field>}} for a field or system field of the
// record, {{type}} for the name of its type, {{title}} for its title, {{link}} for its address
// in the web interface, {{actor}} for who caused the event, {{message}} for the text of a
// rule's notify, {{entry}} for the text of a new entry of the timeline (PL42).
var placeholderRe = regexp.MustCompile(`\{\{\s*([a-z_]+(?:\.[a-z_0-9]+)?)\s*\}\}`)

var plain = map[string]bool{"type": true, "title": true, "link": true, "actor": true, "message": true, "entry": true}

// systemNames are the system fields a placeholder may name.
var systemNames = map[string]bool{"id": true, "version": true, "state": true, "org_path": true, "created_at": true, "updated_at": true}

// checkPlaceholders reports the first placeholder of a text that does not exist.
func checkPlaceholders(t meta.Type, text string) string {
	for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if plain[name] {
			continue
		}
		field, ok := strings.CutPrefix(name, "record.")
		if !ok {
			return fmt.Sprintf("{{%s}} is no placeholder; use {{record.<field>}}, {{type}}, {{title}}, {{link}}, {{actor}}, {{message}} or {{entry}}", name)
		}
		if _, ok := t.Field(field); !ok && !systemNames[field] {
			return fmt.Sprintf("{{%s}}: %s has no field %q", name, t.QualifiedName(), field)
		}
	}
	return ""
}

// Data is what the placeholders of one mail stand for.
type Data struct {
	Type    meta.Type
	Record  records.Record
	Lang    string
	Link    string
	Actor   string
	Message string
	Entry   string
}

// Render fills in the placeholders of a text; an unknown one (in a translation, which the
// check of the record does not see) stays as it is.
func Render(text string, d Data) string {
	return placeholderRe.ReplaceAllStringFunc(text, func(p string) string {
		name := placeholderRe.FindStringSubmatch(p)[1]
		switch name {
		case "type":
			if l := d.Type.Label[d.Lang]; l != "" {
				return l
			}
			return d.Type.Label["de"]
		case "title":
			return str(d.Record[d.Type.TitleField])
		case "link":
			return d.Link
		case "actor":
			return d.Actor
		case "message":
			return d.Message
		case "entry":
			return d.Entry
		}
		if field, ok := strings.CutPrefix(name, "record."); ok {
			if v, ok := d.Record[field]; ok {
				return valueText(d.Type, field, v, d.Lang)
			}
		}
		return p
	})
}

// valueText is a value as a mail shows it: the label of a choice in the language, empty for
// nothing.
func valueText(t meta.Type, field string, v any, lang string) string {
	if f, ok := t.Field(field); ok && f.Type == meta.FieldText && f.Format == meta.FormatRich && v != nil {
		// Mails are plain text: formatted text without its markup, like an entry (UI7, SD10).
		return richtext.Text(str(v))
	}
	if f, ok := t.Field(field); ok && f.Type == meta.FieldSequence && v != nil {
		// The running number with its prefix (PL49), as in the subject "INC-24817".
		return f.Prefix + str(v)
	}
	if f, ok := t.Field(field); ok && f.Type == meta.FieldChoice {
		for _, o := range f.Options {
			if o.Value == fmt.Sprint(v) {
				if l := o.Label[lang]; l != "" {
					return l
				}
				return o.Label["de"]
			}
		}
	}
	return str(v)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
