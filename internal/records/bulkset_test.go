// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// The set-based batch (PL32) writes what Create and Update write: values, source and key,
// history and events; an unchanged row writes nothing; faulty rows get their error.
func TestSyncBatch(t *testing.T) {
	s, ctx, pool := setup(t)
	source := fmt.Sprintf("batch-%d", uuid.New().ID())
	count := func(table, typeName string, ids ...uuid.UUID) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE type_name = $1 AND record_id = ANY($2)", typeName, ids).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	rows := []BulkRow{
		{Key: "a", Hash: []byte("a1"), Fields: map[string]any{"title": "batch a", "due_on": "2026-10-28", "seen_at": "2026-10-28T09:30:00Z", "kind": "b"}},
		{Key: "b", Hash: []byte("b1"), Fields: map[string]any{"title": "batch b", "priority": 2.0, "active": false}},
		{Key: "c", Hash: []byte("c1"), Fields: map[string]any{"title": "batch c", "priority": 9.0}},
		{Key: "d", Hash: []byte("d1"), Fields: map[string]any{"title": "batch d", "parent": uuid.NewString()}},
	}
	res, err := s.SyncBatch(ctx, testType, source, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := s.rowByRow.Load(); n != 0 {
		t.Fatalf("%d rows went row by row", n)
	}
	if res[0].Status != SyncCreated || res[1].Status != SyncCreated {
		t.Fatalf("created: %+v", res)
	}
	for i := 2; i < 4; i++ {
		if _, ok := AsValidationError(res[i].Err); !ok {
			t.Fatalf("row %d: %+v", i, res[i])
		}
	}
	a, err := s.Get(ctx, testType, res[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if a["due_on"] != "2026-10-28" || a["seen_at"] != "2026-10-28T09:30:00Z" || a["kind"] != "b" || a["priority"] != 3.0 ||
		a["active"] != true || a["source"] != source || a["external_key"] != "a" || a["version"] != int64(1) {
		t.Fatalf("values of a: %v", a)
	}
	b, _ := s.Get(ctx, testType, res[1].ID)
	if b["priority"] != 2.0 || b["active"] != false {
		t.Fatalf("values of b: %v", b)
	}
	ids := []uuid.UUID{res[0].ID, res[1].ID}
	// create plus title, due_on, seen_at, kind, priority, active for a; create plus title,
	// priority, active for b; one event each.
	if n := count("audit", testType, ids...); n != 11 {
		t.Fatalf("audit rows: %d", n)
	}
	if n := count("event", testType, ids...); n != 2 {
		t.Fatalf("events: %d", n)
	}

	// The same hashes: nothing written.
	res, err = s.SyncBatch(ctx, testType, source, rows[:2], nil)
	if err != nil || res[0].Status != SyncUnchanged || res[1].Status != SyncUnchanged || res[0].ID != ids[0] {
		t.Fatalf("unchanged: %+v %v", res, err)
	}
	// Another hash: a changes one field, b delivers the same values (only the hash is kept).
	res, err = s.SyncBatch(ctx, testType, source, []BulkRow{
		{Key: "a", Hash: []byte("a2"), Fields: map[string]any{"title": "batch a", "priority": 5.0}},
		{Key: "b", Hash: []byte("b2"), Fields: map[string]any{"title": "batch b", "priority": 2.0, "active": false}},
	}, nil)
	if err != nil || res[0].Status != SyncUpdated || res[1].Status != SyncUnchanged {
		t.Fatalf("second run: %+v %v", res, err)
	}
	a, _ = s.Get(ctx, testType, ids[0])
	if a["priority"] != 5.0 || a["version"] != int64(2) || a["kind"] != "b" || a["due_on"] != "2026-10-28" {
		t.Fatalf("a after the update: %v", a)
	}
	if n := count("audit", testType, ids...); n != 12 {
		t.Fatalf("audit rows after the update: %d", n)
	}
	if n := count("event", testType, ids...); n != 3 {
		t.Fatalf("events after the update: %d", n)
	}
	var hash []byte
	if err := pool.QueryRow(ctx, "SELECT row_hash FROM rec_recordstest_thing WHERE id = $1", ids[1]).Scan(&hash); err != nil || string(hash) != "b2" {
		t.Fatalf("hash of b: %q %v", hash, err)
	}

	// A unique value twice in one batch: the first row is written, the second gets the error.
	res, err = s.SyncBatch(ctx, testType, source, []BulkRow{
		{Key: "e", Hash: []byte("e1"), Fields: map[string]any{"title": "batch same"}},
		{Key: "f", Hash: []byte("f1"), Fields: map[string]any{"title": "batch same"}},
	}, nil)
	if err != nil || res[0].Status != SyncCreated || res[1].Err == nil {
		t.Fatalf("unique twice: %+v %v", res, err)
	}
	// Only the second row went row by row.
	if n := s.rowByRow.Load(); n != 1 {
		t.Fatalf("%d rows went row by row", n)
	}

	// A record in the trash is a conflict of its row.
	if err := s.Trash(ctx, testType, ids[1], nil); err != nil {
		t.Fatal(err)
	}
	res, err = s.SyncBatch(ctx, testType, source, []BulkRow{{Key: "b", Hash: []byte("b3"), Fields: map[string]any{"title": "batch b"}}}, nil)
	if err != nil || !errors.Is(res[0].Err, ErrConflict) {
		t.Fatalf("trashed: %+v %v", res, err)
	}
}
