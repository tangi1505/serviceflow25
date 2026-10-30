// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"encoding/json"
	"errors"
	"testing"
)

// The JSON query of PL14: and / or / not with the operators, paths through references,
// field selection, sorting with key-set paging, aggregates, the trash left out, and bad
// queries refused with a problem list.
func TestQuery(t *testing.T) {
	s, ctx, _ := setup(t)
	mk := func(title string, prio int, kind string, parent any, due any) Record {
		t.Helper()
		in := map[string]any{"title": title, "priority": prio, "kind": kind}
		if parent != nil {
			in["parent"] = parent
		}
		if due != nil {
			in["due_on"] = due
		}
		rec, err := s.Create(ctx, testType, in, nil)
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	a := mk("alpha", 1, "a", nil, nil)
	b := mk("bravo", 2, "b", a["id"], nil)
	c := mk("charlie", 3, "a", a["id"], nil)
	d := mk("delta", 4, "b", nil, nil)
	e := mk("echo", 5, "a", nil, "2026-11-01")
	f := mk("foxtrot", 5, "b", nil, nil)
	if err := s.Trash(ctx, testType, id(t, f), nil); err != nil {
		t.Fatal(err)
	}

	run := func(body string) ([]Record, string) {
		t.Helper()
		var q Query
		if err := json.Unmarshal([]byte(body), &q); err != nil {
			t.Fatal(err)
		}
		page, rows, err := s.Query(ctx, testType, q)
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if rows != nil {
			t.Fatalf("%s: rows, not records", body)
		}
		return page.Items, page.Next
	}
	titles := func(items []Record) []string {
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, it["title"].(string))
		}
		return out
	}
	same := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	// Everything but the trash, in id order.
	if items, _ := run(`{}`); !same(titles(items), []string{"alpha", "bravo", "charlie", "delta", "echo"}) {
		t.Fatalf("all: %v", titles(items))
	}
	if items, _ := run(`{"where": {"and": [{"field": "kind", "eq": "a"}, {"field": "priority", "gte": 3}]}}`); !same(titles(items), []string{"charlie", "echo"}) {
		t.Fatalf("and: %v", titles(items))
	}
	if items, _ := run(`{"where": {"or": [{"field": "title", "contains": "RAV"}, {"field": "priority", "lt": 2}]}}`); !same(titles(items), []string{"alpha", "bravo"}) {
		t.Fatalf("or/contains: %v", titles(items))
	}
	if items, _ := run(`{"where": {"not": {"field": "kind", "in": ["a"]}}}`); !same(titles(items), []string{"bravo", "delta"}) {
		t.Fatalf("not/in: %v", titles(items))
	}
	if items, _ := run(`{"where": {"field": "due_on", "null": false}}`); !same(titles(items), []string{"echo"}) {
		t.Fatalf("null: %v", titles(items))
	}
	if items, _ := run(`{"where": {"field": "parent", "eq": "` + a["id"].(string) + `"}}`); !same(titles(items), []string{"bravo", "charlie"}) {
		t.Fatalf("reference eq: %v", titles(items))
	}
	// A path through the reference, in the condition and in the fields.
	items, _ := run(`{"where": {"field": "parent.title", "starts": "al"}, "fields": ["title", "parent.title", "parent.priority"]}`)
	if !same(titles(items), []string{"bravo", "charlie"}) || items[0]["parent.title"] != "alpha" || items[0]["parent.priority"] != 1.0 || items[0]["kind"] != nil {
		t.Fatalf("path: %v", items)
	}
	// Sorting with pages of two: the cursor continues where the page ended.
	page1, next := run(`{"sort": ["-priority"], "limit": 2}`)
	if !same(titles(page1), []string{"echo", "delta"}) || next == "" {
		t.Fatalf("page 1: %v", titles(page1))
	}
	page2, next2 := run(`{"sort": ["-priority"], "limit": 2, "after": "` + next + `"}`)
	if !same(titles(page2), []string{"charlie", "bravo"}) || next2 == "" {
		t.Fatalf("page 2: %v", titles(page2))
	}
	page3, next3 := run(`{"sort": ["-priority"], "limit": 2, "after": "` + next2 + `"}`)
	if !same(titles(page3), []string{"alpha"}) || next3 != "" {
		t.Fatalf("page 3: %v %q", titles(page3), next3)
	}
	// Nulls last: due_on is set on echo only.
	if items, _ := run(`{"sort": ["due_on"], "limit": 10}`); titles(items)[0] != "echo" {
		t.Fatalf("nulls last: %v", titles(items))
	}
	_ = b
	_ = c
	_ = d
	_ = e

	// Aggregates: per kind the count and the sum of priorities.
	var q Query
	_ = json.Unmarshal([]byte(`{"group": ["kind"], "aggregate": {"n": {"count": "*"}, "p": {"sum": "priority"}, "top": {"max": "priority"}}}`), &q)
	_, rows, err := s.Query(ctx, testType, q)
	if err != nil || rows == nil || len(rows.Rows) != 2 {
		t.Fatalf("aggregate: %v %v", rows, err)
	}
	if r := rows.Rows[0]; r["kind"] != "a" || r["n"] != 3.0 || r["p"] != 9.0 || r["top"] != 5.0 {
		t.Fatalf("group a: %v", r)
	}
	if r := rows.Rows[1]; r["kind"] != "b" || r["n"] != 2.0 || r["p"] != 6.0 {
		t.Fatalf("group b: %v", r)
	}
	var q2 Query // a fresh one: Unmarshal keeps fields the JSON does not mention
	_ = json.Unmarshal([]byte(`{"where": {"field": "parent.title", "eq": "alpha"}, "aggregate": {"n": {"count": "*"}}}`), &q2)
	if _, rows, err := s.Query(ctx, testType, q2); err != nil || len(rows.Rows) != 1 || rows.Rows[0]["n"] != 2.0 {
		t.Fatalf("count with path: %v %v", rows, err)
	}

	// Refused: an unknown operator, a relation (PL15), a path over a text field, a bad value.
	for _, bad := range []string{
		`{"where": {"field": "title", "like": "x"}}`,
		`{"where": {"rel": "affects", "to": "ci", "where": {}}}`,
		`{"where": {"field": "title.length", "eq": 1}}`,
		`{"where": {"field": "priority", "eq": "high"}}`,
		`{"where": {"field": "nope", "eq": 1}}`,
		`{"sort": ["parent.title"]}`,
		`{"aggregate": {"s": {"sum": "title"}}}`,
	} {
		var bq Query
		_ = json.Unmarshal([]byte(bad), &bq)
		_, _, err := s.Query(ctx, testType, bq)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s must be refused with problems, got %v", bad, err)
		}
	}
}
