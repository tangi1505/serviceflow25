// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package savedsearch checks the saved searches (UI4; Inventar G112; Hauptplan E124). A saved
// search is a record of the core type core.saved_search; sharing is its role "shared" (WF3),
// so reading and writing go through the records and the rights like any type. This package
// only refuses what could not run: an unknown type, a condition the JSON query does not take,
// a column or a sort the type does not have.
package savedsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// TypeSavedSearch is the core type of the saved searches.
const TypeSavedSearch = "core.saved_search"

// systemColumns are the columns every record has besides its fields.
var systemColumns = map[string]bool{"id": true, "version": true, "state": true, "org_path": true, "created_at": true, "updated_at": true}

// Check is the RecordCheck of the saved searches; rec runs the condition once (one row) with
// the rights of the writer, so that a wrong field or operator is refused when saving.
func Check(rec *records.Service) records.RecordCheck {
	return func(ctx context.Context, t meta.Type, r map[string]any) []records.Problem {
		if t.QualifiedName() != TypeSavedSearch {
			return nil
		}
		var ps []records.Problem
		add := func(field, code, msg string) {
			ps = append(ps, records.Problem{Field: field, Code: code, Message: msg})
		}
		name, _ := r["target_type"].(string)
		target, err := rec.Type(ctx, name)
		if err != nil {
			add("target_type", "unknown", fmt.Sprintf("type %s is not installed", name))
			return ps
		}
		if q, _ := r["query"].(string); strings.TrimSpace(q) != "" {
			var where map[string]any
			if err := json.Unmarshal([]byte(q), &where); err != nil {
				add("query", "type", "must be the where of a JSON query, an object like {\"field\": \"state\", \"eq\": \"new\"}")
			} else if _, _, err := rec.Query(ctx, name, records.Query{Where: json.RawMessage(q), Limit: 1}); err != nil {
				var verr *records.ValidationError
				if !errors.As(err, &verr) {
					return append(ps, records.Problem{Field: "query", Code: "invalid", Message: err.Error()})
				}
				for _, p := range verr.Problems {
					add("query", p.Code, fmt.Sprintf("%s: %s", p.Field, p.Message))
				}
			}
		}
		known := func(col string) bool {
			_, ok := target.Field(col)
			return ok || systemColumns[col] || (col == "actors" && len(target.Actors) > 0)
		}
		if cols, _ := r["columns"].(string); cols != "" {
			for _, c := range strings.Split(cols, ",") {
				if c = strings.TrimSpace(c); c != "" && !known(c) {
					add("columns", "unknown", fmt.Sprintf("%s has no column %q", name, c))
				}
			}
		}
		if s, _ := r["sort"].(string); s != "" && !known(strings.TrimPrefix(s, "-")) {
			add("sort", "unknown", fmt.Sprintf("%s has no column %q to sort by", name, strings.TrimPrefix(s, "-")))
		}
		return ps
	}
}
