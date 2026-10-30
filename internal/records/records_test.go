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
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const testType = "recordstest.thing"

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// setup installs the package recordstest with one type and returns a fresh service.
func setup(t *testing.T) (*Service, context.Context, *pgxpool.Pool) {
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
		"DROP TABLE IF EXISTS rec_recordstest_thing",
		"DELETE FROM package WHERE name = 'recordstest'",
		"DELETE FROM audit WHERE type_name = 'recordstest.thing'",
		"DELETE FROM schema_change WHERE type_name = 'recordstest.thing'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml": {Data: []byte("name: recordstest\nversion: 0.1.0\nlabel: {de: T, en: T}\n")},
		"types/thing.yaml": {Data: []byte(`name: thing
label: {de: Ding, en: Thing}
traits: [history]
fields:
  - {name: title, type: text, label: {de: T, en: T}, required: true, max_length: 20, unique: true}
  - {name: priority, type: number, label: {de: P, en: P}, min: 1, max: 5, default: 3}
  - {name: active, type: boolean, label: {de: A, en: A}, default: true}
  - {name: due_on, type: date, label: {de: D, en: D}}
  - {name: seen_at, type: datetime, label: {de: S, en: S}}
  - {name: kind, type: choice, label: {de: K, en: K}, options: [{value: a, label: {de: A, en: A}}, {value: b, label: {de: B, en: B}}]}
  - {name: parent, type: reference, label: {de: R, en: R}, to: recordstest.thing}
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	return New(pool), ctx, pool
}

func id(t *testing.T, rec Record) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(rec["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateAppliesDefaultsAndWritesAudit(t *testing.T) {
	s, ctx, _ := setup(t)
	rec, err := s.Create(ctx, testType, map[string]any{"title": "first", "due_on": "2026-10-20", "seen_at": "2026-10-20T10:00:00Z", "kind": "a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec["version"] != int64(1) || rec["priority"] != 3.0 || rec["active"] != true || rec["due_on"] != "2026-10-20" ||
		rec["seen_at"] != "2026-10-20T10:00:00Z" || rec["kind"] != "a" || rec["parent"] != nil || rec["deleted_at"] != nil {
		t.Fatalf("unexpected record: %v", rec)
	}
	hist, err := s.History(ctx, testType, id(t, rec))
	if err != nil {
		t.Fatal(err)
	}
	// create + one update row per set field (title, priority, active, due_on, seen_at, kind)
	if len(hist) != 7 || hist[0].Action != "create" {
		t.Fatalf("history has %d rows: %+v", len(hist), hist)
	}
}

func TestValidationNamesEveryProblem(t *testing.T) {
	s, ctx, _ := setup(t)
	_, err := s.Create(ctx, testType, map[string]any{
		"priority": 9, "kind": "zzz", "due_on": "20.10.2026", "colour": "red", "version": 5,
		"parent": uuid.Nil.String(),
	}, nil)
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected validation error, got %v", err)
	}
	got := map[string]string{}
	for _, p := range ve.Problems {
		got[p.Field] = p.Code
	}
	want := map[string]string{"title": "required", "priority": "max", "kind": "choice", "due_on": "type", "colour": "unknown", "version": "read_only", "parent": "reference"}
	for f, code := range want {
		if got[f] != code {
			t.Errorf("field %s: got %q, want %q (all: %v)", f, got[f], code, got)
		}
	}
}

func TestUpdateConflictAndAuditPerField(t *testing.T) {
	s, ctx, _ := setup(t)
	rec, err := s.Create(ctx, testType, map[string]any{"title": "a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rid := id(t, rec)

	// Two writers saw version 1; the second must fail.
	r2, err := s.Update(ctx, testType, rid, 1, map[string]any{"title": "b", "priority": 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r2["version"] != int64(2) || r2["title"] != "b" || r2["priority"] != 4.0 {
		t.Fatalf("unexpected after update: %v", r2)
	}
	_, err = s.Update(ctx, testType, rid, 1, map[string]any{"title": "c"}, nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version must conflict, got %v", err)
	}
	// Unchanged value: no new version, no audit.
	r3, err := s.Update(ctx, testType, rid, 2, map[string]any{"title": "b"}, nil)
	if err != nil || r3["version"] != int64(2) {
		t.Fatalf("unchanged update: %v %v", err, r3)
	}
	// Clearing a field and setting a reference.
	other, err := s.Create(ctx, testType, map[string]any{"title": "other"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r4, err := s.Update(ctx, testType, rid, 2, map[string]any{"priority": nil, "parent": other["id"]}, nil)
	if err != nil || r4["priority"] != nil || r4["parent"] != other["id"] || r4["version"] != int64(3) {
		t.Fatalf("clear and reference: %v %v", err, r4)
	}

	hist, err := s.History(ctx, testType, rid)
	if err != nil {
		t.Fatal(err)
	}
	var v2, v3 int
	for _, h := range hist {
		switch h.Version {
		case 2:
			v2++
		case 3:
			v3++
		}
	}
	if v2 != 2 || v3 != 2 {
		t.Fatalf("audit rows per changed field: version 2 has %d (want 2), version 3 has %d (want 2)", v2, v3)
	}
	for _, h := range hist {
		if h.Version == 2 && h.Field.Valid && h.Field.String == "title" && (string(h.OldValue) != `"a"` || string(h.NewValue) != `"b"`) {
			t.Fatalf("title audit: %s -> %s", h.OldValue, h.NewValue)
		}
	}
}

func TestTrashRestorePurgeAndList(t *testing.T) {
	s, ctx, _ := setup(t)
	var ids []uuid.UUID
	for _, title := range []string{"one", "two", "three"} {
		rec, err := s.Create(ctx, testType, map[string]any{"title": title}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id(t, rec))
	}
	if err := s.Trash(ctx, testType, ids[1], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, testType, ids[1]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("trashed record must not be readable, got %v", err)
	}
	page, err := s.List(ctx, testType, ListOptions{})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("list after trash: %v, %d items", err, len(page.Items))
	}
	if err := s.Trash(ctx, testType, ids[1], nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("trashing twice must conflict, got %v", err)
	}
	// A duplicate title while "two" is in the trash is allowed (E4) …
	dup, err := s.Create(ctx, testType, map[string]any{"title": "two"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// … and then the restore must fail as a conflict naming the field.
	if err := s.Restore(ctx, testType, ids[1], nil); !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "title") {
		t.Fatalf("restore onto a duplicate must conflict on title, got %v", err)
	}
	if err := s.Trash(ctx, testType, id(t, dup), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, testType, id(t, dup), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(ctx, testType, ids[1], nil); err != nil {
		t.Fatalf("restore after purge of the duplicate: %v", err)
	}
	if err := s.Purge(ctx, testType, ids[0], nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("purge outside the trash must conflict, got %v", err)
	}
	// Key-set paging.
	p1, err := s.List(ctx, testType, ListOptions{Limit: 2})
	if err != nil || len(p1.Items) != 2 || p1.Next == "" {
		t.Fatalf("page 1: %v %+v", err, p1)
	}
	p2, err := s.List(ctx, testType, ListOptions{Limit: 2, After: p1.Next})
	if err != nil || len(p2.Items) != 1 || p2.Next != "" {
		t.Fatalf("page 2: %v %+v", err, p2)
	}
	// Descending by created_at with a composite cursor, filter on a field, field selection.
	d1, err := s.List(ctx, testType, ListOptions{Limit: 2, Sort: "-created_at", Fields: []string{"title"}})
	if err != nil || len(d1.Items) != 2 || d1.Items[0]["title"] != "three" || d1.Next == "" || d1.Items[0]["priority"] != nil {
		t.Fatalf("desc page 1: %v %+v", err, d1)
	}
	d2, err := s.List(ctx, testType, ListOptions{Limit: 2, Sort: "-created_at", Fields: []string{"title"}, After: d1.Next})
	if err != nil || len(d2.Items) != 1 || d2.Items[0]["title"] != "one" {
		t.Fatalf("desc page 2: %v %+v", err, d2)
	}
	fl, err := s.List(ctx, testType, ListOptions{Filter: map[string]string{"title": "two", "priority": "3"}})
	if err != nil || len(fl.Items) != 1 || fl.Items[0]["title"] != "two" {
		t.Fatalf("filter: %v %+v", err, fl)
	}
	_, err = s.List(ctx, testType, ListOptions{Sort: "title", Filter: map[string]string{"colour": "x", "priority": "abc"}, After: "nope"})
	ve, ok := AsValidationError(err)
	if !ok || len(ve.Problems) != 4 {
		t.Fatalf("list options must be validated: %v", err)
	}
	if _, err := s.Get(ctx, "recordstest.nope", ids[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown type: %v", err)
	}
}
