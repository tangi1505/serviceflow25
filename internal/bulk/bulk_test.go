// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"group24.de/itsm/internal/records"
)

// fakeWriter keeps the hash per key and answers like records.Service.Sync.
type fakeWriter struct {
	hashes map[string][]byte
	fail   map[string]error
	got    []map[string]any
}

func (f *fakeWriter) Sync(_ context.Context, _ string, key records.SyncKey, fields map[string]any, _ records.Actor) (uuid.UUID, int64, string, error) {
	f.got = append(f.got, fields)
	if err := f.fail[key.Key]; err != nil {
		return uuid.Nil, 0, "", err
	}
	old, known := f.hashes[key.Key]
	f.hashes[key.Key] = key.Hash
	switch {
	case !known:
		return uuid.New(), 1, records.SyncCreated, nil
	case bytes.Equal(old, key.Hash):
		return uuid.New(), 1, records.SyncUnchanged, nil
	}
	return uuid.New(), 2, records.SyncUpdated, nil
}

func TestSyncRows(t *testing.T) {
	w := &fakeWriter{hashes: map[string][]byte{}, fail: map[string]error{
		"forbidden": records.ErrForbidden,
		"invalid":   &records.ValidationError{Problems: []records.Problem{{Field: "priority", Code: "max", Message: "at most 5"}}},
	}}
	rows := []map[string]any{
		{"external_key": "a", "title": "A"},
		{"title": "no key"},
		{"external_key": "forbidden", "title": "F"},
		{"external_key": "invalid", "priority": 9},
		{"external_key": " b ", "title": "B"},
	}
	out, err := Sync(t.Context(), w, "probe.probe", "flowzilla", rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Created != 2 || out.Errors != 3 || len(out.Items) != 5 {
		t.Fatalf("counts: %+v", out)
	}
	if r := out.Items[1]; r.Status != StatusError || r.Code != "validation" || r.Problems[0].Field != KeyField {
		t.Errorf("row without key: %+v", r)
	}
	if r := out.Items[2]; r.Code != "forbidden" || r.ID != nil {
		t.Errorf("forbidden row: %+v", r)
	}
	if r := out.Items[3]; r.Code != "validation" || r.Problems[0].Field != "priority" {
		t.Errorf("invalid row: %+v", r)
	}
	if r := out.Items[4]; r.ExternalKey != "b" || r.Status != records.SyncCreated || r.Index != 4 {
		t.Errorf("trimmed key: %+v", r)
	}
	// The key is not a field.
	if _, ok := w.got[0][KeyField]; ok {
		t.Errorf("external_key passed on as a field: %v", w.got[0])
	}

	// The same rows again: unchanged; a changed field: updated.
	rows[0]["title"] = "A2"
	out, err = Sync(t.Context(), w, "probe.probe", "flowzilla", rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Updated != 1 || out.Unchanged != 1 || out.Items[0].Status != records.SyncUpdated || out.Items[4].Status != records.SyncUnchanged {
		t.Fatalf("second run: %+v", out)
	}
}

func TestSyncRefusesTheCall(t *testing.T) {
	w := &fakeWriter{hashes: map[string][]byte{}}
	many := make([]map[string]any, MaxRows+1)
	for name, c := range map[string]struct {
		source string
		rows   []map[string]any
		field  string
	}{
		"no source":     {"  ", []map[string]any{{"external_key": "a"}}, "source"},
		"long source":   {strings.Repeat("s", 101), []map[string]any{{"external_key": "a"}}, "source"},
		"no rows":       {"flowzilla", nil, "rows"},
		"too many rows": {"flowzilla", many, "rows"},
	} {
		_, err := Sync(t.Context(), w, "probe.probe", c.source, c.rows, nil)
		var invalid *InvalidError
		if !errors.As(err, &invalid) || invalid.Field != c.field {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(w.got) != 0 {
		t.Errorf("rows written of a refused call: %d", len(w.got))
	}
}

// An error of the database is not the row's fault: the call stops.
func TestSyncStopsOnDatabaseErrors(t *testing.T) {
	w := &fakeWriter{hashes: map[string][]byte{}, fail: map[string]error{"b": errors.New("connection refused")}}
	_, err := Sync(t.Context(), w, "probe.probe", "flowzilla", []map[string]any{{"external_key": "a"}, {"external_key": "b"}, {"external_key": "c"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "row 1") || len(w.got) != 2 {
		t.Fatalf("err %v after %d rows", err, len(w.got))
	}
}

func TestHash(t *testing.T) {
	var a, b map[string]any
	_ = json.Unmarshal([]byte(`{"title": "x", "priority": 2, "tags": ["a"]}`), &a)
	_ = json.Unmarshal([]byte(`{"tags": ["a"], "priority": 2, "title": "x"}`), &b)
	if !bytes.Equal(Hash(a), Hash(b)) {
		t.Error("the order of the keys changes the hash")
	}
	b["title"] = "y"
	if bytes.Equal(Hash(a), Hash(b)) {
		t.Error("another value, the same hash")
	}
	delete(b, "title")
	if bytes.Equal(Hash(a), Hash(b)) {
		t.Error("one field fewer, the same hash")
	}
}
