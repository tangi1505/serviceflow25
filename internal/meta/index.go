// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"fmt"
	"strings"
)

// Index is an index a package declares for the frequent lists of its type (PL54, Kern-Plan
// E65): its columns in order. The core indexes the moment of creation of every type itself.
type Index struct {
	Fields []string `json:"fields" yaml:"fields"`
}

// MaxIndexFields is the most columns one declared index takes.
const MaxIndexFields = 4

// MaxIndexedText is the longest text field an index takes: a B-tree entry holds about 2,700
// bytes, a character takes up to four.
const MaxIndexedText = 500

// indexableSystem are the system columns a declared index may name besides the fields.
var indexableSystem = map[string]bool{
	"state": true, "created_at": true, "created_by": true, "updated_at": true, "updated_by": true,
	"sla_due_at": true, "source": true, "external_key": true,
}

// validateIndexes checks the declared indexes: one to four columns, each a field of the type
// or an indexable system column, named once; text fields with a short maximum length.
func validateIndexes(add func(string, string, ...any), where string, t Type) {
	seen := map[string]bool{}
	for i, ix := range t.Indexes {
		iw := fmt.Sprintf("%s, index %d", where, i+1)
		if len(ix.Fields) == 0 || len(ix.Fields) > MaxIndexFields {
			add(iw, "needs 1 to %d fields, has %d", MaxIndexFields, len(ix.Fields))
		}
		named := map[string]bool{}
		for _, name := range ix.Fields {
			if named[name] {
				add(iw, "names %q twice", name)
			}
			named[name] = true
			if indexableSystem[name] {
				continue
			}
			if isSystemColumn(name) {
				add(iw, "system column %q cannot be indexed by a package", name)
				continue
			}
			f, ok := t.Field(name)
			if !ok {
				add(iw, "%q is not a field of the type", name)
				continue
			}
			if f.Type == FieldText && (f.MaxLength == 0 || f.MaxLength > MaxIndexedText) {
				add(iw, "text field %q needs a max_length of at most %d to be indexed", name, MaxIndexedText)
			}
		}
		key := strings.Join(ix.Fields, ",")
		if seen[key] {
			add(iw, "is declared twice")
		}
		seen[key] = true
	}
}
