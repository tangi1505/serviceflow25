// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package notify

import (
	"fmt"
	"html"
	"regexp"
	"slices"
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

var plain = []string{"type", "title", "link", "actor", "message", "entry"}

// systemNames are the system fields a placeholder may name.
var systemNames = map[string]bool{"id": true, "version": true, "state": true, "org_path": true, "created_at": true, "updated_at": true}

// checkPlaceholders reports the first placeholder of a text that does not exist.
func checkPlaceholders(t meta.Type, text string) string {
	return CheckPlaceholders(text, plain, t.QualifiedName(), func(field string) bool {
		_, ok := t.Field(field)
		return ok
	})
}

// CheckPlaceholders reports the first placeholder of a text that is neither one of names nor
// {{record.<field>}} of a field hasField knows or a system field; where names the type in the
// message, empty for all installed types. The text modules (SD8) take it with their own names.
func CheckPlaceholders(text string, names []string, where string, hasField func(string) bool) string {
	for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if slices.Contains(names, name) {
			continue
		}
		field, ok := strings.CutPrefix(name, "record.")
		if !ok {
			use := make([]string, len(names))
			for i, n := range names {
				use[i] = "{{" + n + "}}"
			}
			return fmt.Sprintf("{{%s}} is no placeholder; use {{record.<field>}}, %s or %s",
				name, strings.Join(use[:len(use)-1], ", "), use[len(use)-1])
		}
		if !hasField(field) && !systemNames[field] {
			if where == "" {
				return fmt.Sprintf("{{%s}}: no installed type has a field %q", name, field)
			}
			return fmt.Sprintf("{{%s}}: %s has no field %q", name, where, field)
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
	// HTML fills the placeholders of formatted text (the text modules, SD8): the values are
	// escaped and {{link}} becomes a link.
	HTML bool
	// Name gives the title of a referenced record; without it a reference stays its id.
	Name func(typeName, id string) string
}

// Render fills in the placeholders of a text; an unknown one (in a translation, which the
// check of the record does not see) stays as it is.
func Render(text string, d Data) string {
	return placeholderRe.ReplaceAllStringFunc(text, func(p string) string {
		name := placeholderRe.FindStringSubmatch(p)[1]
		if name == "link" && d.HTML {
			l := html.EscapeString(d.Link)
			return `<a href="` + l + `">` + l + `</a>`
		}
		v, ok := d.value(name)
		if !ok {
			return p
		}
		if d.HTML {
			return html.EscapeString(v)
		}
		return v
	})
}

// value is the text of one placeholder; false for one the record does not have.
func (d Data) value(name string) (string, bool) {
	switch name {
	case "type":
		if l := d.Type.Label[d.Lang]; l != "" {
			return l, true
		}
		return d.Type.Label["de"], true
	case "title":
		return str(d.Record[d.Type.TitleField]), true
	case "link":
		return d.Link, true
	case "actor":
		return d.Actor, true
	case "message":
		return d.Message, true
	case "entry":
		return d.Entry, true
	}
	field, ok := strings.CutPrefix(name, "record.")
	if !ok {
		return "", false
	}
	v, ok := d.Record[field]
	if !ok {
		return "", false
	}
	if f, ok := d.Type.Field(field); ok && f.Type == meta.FieldReference && d.Name != nil && v != nil {
		return d.Name(f.To, str(v)), true
	}
	return valueText(d.Type, field, v, d.Lang), true
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
