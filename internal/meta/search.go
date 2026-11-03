// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

// --- full-text search (Hauptplan E21, E101; PL26) ---

// SearchFields are the fields that make up the search column of a type: the title field (it
// weighs most and carries the trigram index for typos) and the other searchable fields. A
// type with neither is not searched.
func (t Type) SearchFields() (title string, others []string) {
	if f, ok := t.Field(t.TitleField); ok && f.Type == FieldText {
		title = f.Name
	}
	for _, f := range t.Fields {
		// A running number is always found, with and without its prefix (PL49).
		if (f.Searchable || f.Type == FieldSequence) && f.Name != title {
			others = append(others, f.Name)
		}
	}
	return title, others
}

// Searched reports whether the type takes part in the search.
func (t Type) Searched() bool {
	title, others := t.SearchFields()
	return title != "" || len(others) > 0
}
