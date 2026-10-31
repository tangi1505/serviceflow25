// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const stateType = "statestest.case"

const caseYAML = `name: case
label: {de: Fall, en: Case}
traits: [history, actors]
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
  - {name: solution, type: text, label: {de: Lösung, en: Solution}}
  - {name: kind, type: choice, label: {de: Art, en: Kind}, default: fault, options: [{value: fault, label: {de: S, en: F}}, {value: wish, label: {de: W, en: W}}]}
  - {name: priority, type: number, label: {de: P, en: P}, default: 3}
  - {name: source_ref, type: text, label: {de: Q, en: S}}
actors:
  - {name: assigned, label: {de: B, en: A}, to: [statestest.case]}
states:
  - {name: open, label: {de: Offen, en: Open}, initial: true}
  - {name: resolved, label: {de: Gelöst, en: Resolved}}
  - {name: closed, label: {de: Geschlossen, en: Closed}}
transitions:
  - {name: resolve, label: {de: L, en: R}, from: [open], to: resolved, when: 'record.title != ""'}
  - {name: close, label: {de: S, en: C}, from: [resolved], to: closed}
  - {name: reopen, label: {de: W, en: R}, from: [closed], to: open}
field_states:
  - {states: [resolved, closed], required: [solution]}
  - {states: [closed], read_only: ["*"]}
  - {read_only: [source_ref]}
  - {when: 'record.kind == "wish"', hidden: [solution], required: [title]}
  - {states: [open], when: 'record.kind == "wish"', defaults: {priority: 5, source_ref: portal}}
`

func setupStates(t *testing.T) (*Service, context.Context) {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_statestest_case",
		"DELETE FROM package WHERE name = 'statestest'",
		"DELETE FROM audit WHERE type_name LIKE 'statestest.%'",
		"DELETE FROM event WHERE type_name LIKE 'statestest.%'",
		"DELETE FROM actor WHERE type_name LIKE 'statestest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'statestest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":    {Data: []byte("name: statestest\nversion: 0.1.0\nlabel: {de: Z, en: S}\n")},
		"types/case.yaml": {Data: []byte(caseYAML)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	return New(db.Single(pool)), ctx
}

// The core of WF2: presets and read-only fields on create, read-only and locked records on
// change, required fields of the target state on a transition, fields set with a transition
// in one version, the form variant through a condition, the same rules in the bulk.
func TestFieldStates(t *testing.T) {
	s, ctx := setupStates(t)

	// Create: a read-only field cannot be set; a wish gets its presets and needs a title.
	if _, err := s.Create(ctx, stateType, map[string]any{"title": "x", "source_ref": "mail"}, nil); problemCodes(err)["source_ref"] != "read_only" {
		t.Fatalf("read-only field on create: %v", err)
	}
	if _, err := s.Create(ctx, stateType, map[string]any{"kind": "wish"}, nil); problemCodes(err)["title"] != "required" {
		t.Fatalf("a wish needs a title: %v", err)
	}
	wish, err := s.Create(ctx, stateType, map[string]any{"kind": "wish", "title": "Monitor"}, nil)
	if err != nil || wish["priority"] != 5.0 || wish["source_ref"] != "portal" {
		t.Fatalf("presets of a wish: %v %v", wish, err)
	}
	if _, err := s.Create(ctx, stateType, map[string]any{"kind": "wish", "title": "x", "source_ref": "portal"}, nil); err != nil {
		t.Fatalf("a read-only field may carry its preset: %v", err)
	}
	fault, err := s.Create(ctx, stateType, map[string]any{"title": "Drucker"}, nil)
	if err != nil || fault["priority"] != 3.0 || fault["source_ref"] != nil {
		t.Fatalf("a fault keeps the defaults of the type: %v %v", fault, err)
	}
	fid := id(t, fault)

	// Change: a read-only field cannot change, its unchanged value can be sent along.
	if _, err := s.Update(ctx, stateType, fid, 1, map[string]any{"source_ref": "x"}, nil); problemCodes(err)["source_ref"] != "read_only" {
		t.Fatalf("read-only field on change: %v", err)
	}
	if rec, err := s.Update(ctx, stateType, fid, 1, map[string]any{"title": "Drucker 2", "source_ref": nil}, nil); err != nil || rec["version"] != int64(2) {
		t.Fatalf("unchanged read-only value along: %v %v", rec, err)
	}

	// Transition: resolved needs a solution (422 with the field), the fields go with it.
	if _, err := s.Transition(ctx, stateType, fid, "resolve", nil, -1, nil); problemCodes(err)["solution"] != "required" {
		t.Fatalf("resolve without solution: %v", err)
	}
	if _, err := s.Transition(ctx, stateType, fid, "resolve", map[string]any{"solution": "Toner"}, 1, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version on a transition: %v", err)
	}
	if _, err := s.Transition(ctx, stateType, fid, "resolve", map[string]any{"title": ""}, 2, nil); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("the condition sees the record as it will be: %v", err)
	}
	rec, err := s.Transition(ctx, stateType, fid, "resolve", map[string]any{"solution": "Toner"}, 2, nil)
	if err != nil || rec["state"] != "resolved" || rec["solution"] != "Toner" || rec["version"] != int64(3) {
		t.Fatalf("resolve with the solution: %v %v", rec, err)
	}
	if _, err := s.Update(ctx, stateType, fid, 3, map[string]any{"solution": nil}, nil); problemCodes(err)["solution"] != "required" {
		t.Fatalf("clearing a required field: %v", err)
	}
	hist, err := s.History(ctx, stateType, fid)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, a := range hist {
		if a.Version == 3 && a.Field.Valid {
			actions[a.Field.String] = a.Action
		}
	}
	if actions["state"] != "transition" || actions["solution"] != "update" {
		t.Fatalf("history of version 3: %v", actions)
	}

	// Closed: the record is locked, fields and roles; the transition back still works.
	if rec, err = s.Transition(ctx, stateType, fid, "close", nil, -1, nil); err != nil || rec["state"] != "closed" {
		t.Fatalf("close: %v %v", rec, err)
	}
	if _, err := s.Update(ctx, stateType, fid, 4, map[string]any{"title": "x", "priority": 1}, nil); problemCodes(err)["title"] != "read_only" || problemCodes(err)["priority"] != "read_only" {
		t.Fatalf("change of a closed record: %v", err)
	}
	if _, err := s.Update(ctx, stateType, fid, 4, map[string]any{"actors": map[string]any{"assigned": []any{map[string]any{"type": stateType, "id": wish["id"]}}}}, nil); problemCodes(err)["actors"] != "read_only" {
		t.Fatalf("roles of a closed record: %v", err)
	}
	if _, err := s.Transition(ctx, stateType, fid, "reopen", map[string]any{"title": "x"}, 4, nil); problemCodes(err)["title"] != "read_only" {
		t.Fatalf("fields with a transition out of a locked state: %v", err)
	}
	if rec, err = s.Transition(ctx, stateType, fid, "reopen", nil, 4, nil); err != nil || rec["state"] != "open" {
		t.Fatalf("reopen: %v %v", rec, err)
	}

	// What the form gets: a closed record is locked; a draft wish hides the solution.
	fs, err := s.FieldStates(ctx, stateType, nil, map[string]any{"kind": "wish"})
	if err != nil || fs.State != "open" || !contains(fs.Hidden, "solution") || !contains(fs.Required, "title") || fs.Defaults["priority"] != 5.0 || fs.Locked {
		t.Fatalf("draft wish: %+v %v", fs, err)
	}
	fs, err = s.FieldStates(ctx, stateType, nil, map[string]any{"kind": "fault"})
	if err != nil || len(fs.Hidden) != 0 || !contains(fs.ReadOnly, "source_ref") || len(fs.Defaults) != 0 {
		t.Fatalf("draft fault: %+v %v", fs, err)
	}

	// The bulk (set-based way) keeps the same rules.
	res, err := s.SyncBatch(ctx, stateType, "statestest", []BulkRow{
		{Key: "a", Fields: map[string]any{"kind": "wish", "title": "Bulk"}, Hash: []byte("a")},
		{Key: "b", Fields: map[string]any{"kind": "wish"}, Hash: []byte("b")},
		{Key: "c", Fields: map[string]any{"title": "x", "source_ref": "y"}, Hash: []byte("c")},
	}, nil)
	if err != nil || res[0].Err != nil || res[1].Err == nil || res[2].Err == nil {
		t.Fatalf("bulk with field states: %+v %v", res, err)
	}
	if got, _ := s.Get(ctx, stateType, res[0].ID); got["priority"] != 5.0 {
		t.Fatalf("bulk wish preset: %v", got)
	}
}

// Rules must name known states and fields, compile, and not contradict themselves.
func TestFieldStatesInTheDefinition(t *testing.T) {
	load := func(rule string) error {
		yaml := strings.Replace(caseYAML, "field_states:\n", "field_states:\n"+rule+"\n", 1)
		p, err := meta.LoadFS(fstest.MapFS{
			"package.yaml":    {Data: []byte("name: statescheck\nversion: 0.1.0\nlabel: {de: Z, en: S}\n")},
			"types/case.yaml": {Data: []byte(strings.ReplaceAll(yaml, "statestest.", "statescheck."))},
		})
		if err != nil {
			return err
		}
		return meta.Validate(p, nil)
	}
	for _, c := range []struct{ rule, want string }{
		{"  - {states: [gone], required: [title]}", "not a state"},
		{"  - {required: [nobody]}", "not a field"},
		{"  - {required: [\"*\"]}", "stands alone"},
		{"  - {read_only: [\"*\", title]}", "stands alone"},
		{"  - {required: [title], hidden: [title]}", "cannot be required and hidden"},
		{"  - {when: 'record.kind ==', required: [title]}", "when"},
		{"  - {defaults: {priority: high}}", "must be a number"},
		{"  - {defaults: {kind: other}}", "not an option"},
		{"  - {states: [open]}", "says nothing"},
	} {
		if err := load(c.rule); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.rule, c.want, err)
		}
	}
}
