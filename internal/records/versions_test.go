// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"errors"
	"strings"
	"testing"
)

// The acceptance of PL19: three changes; comparing version 1 and 3 shows exactly the fields
// that changed between them; every version can be read back; restoring version 1 writes its
// values as version 5.
func TestVersionsCompareAndRestore(t *testing.T) {
	s, ctx, pool := setup(t)
	rec, err := s.Create(ctx, testType, map[string]any{"title": "v1", "kind": "a", "due_on": "2026-10-20"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rid := id(t, rec)
	// Version 2: title and priority; version 3: kind cleared; version 4: title again.
	steps := []map[string]any{
		{"title": "v2", "priority": 5.0},
		{"kind": nil},
		{"title": "v4"},
	}
	for i, patch := range steps {
		if _, err := s.Update(ctx, testType, rid, int64(i+1), patch, nil); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := s.Versions(ctx, testType, rid)
	if err != nil || len(versions) != 4 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	if versions[0].Actions[0] != "create" || strings.Join(versions[1].Changed, ",") != "title,priority" || strings.Join(versions[2].Changed, ",") != "kind" {
		t.Fatalf("version list: %+v", versions)
	}

	cmp, err := s.Compare(ctx, testType, rid, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Change{}
	for _, c := range cmp.Changes {
		got[c.Field] = c
	}
	if len(cmp.Changes) != 3 || got["title"].From != "v1" || got["title"].To != "v2" || got["priority"].From != 3.0 ||
		got["priority"].To != 5.0 || got["kind"].From != "a" || got["kind"].To != nil {
		t.Fatalf("compare 1 and 3: %+v", cmp.Changes)
	}
	if cmp, err = s.Compare(ctx, testType, rid, 3, 3); err != nil || len(cmp.Changes) != 0 {
		t.Fatalf("compare 3 and 3: %+v %v", cmp, err)
	}

	v1, err := s.Version(ctx, testType, rid, 1)
	if err != nil || v1["title"] != "v1" || v1["kind"] != "a" || v1["priority"] != 3.0 || v1["due_on"] != "2026-10-20" || v1["version"] != int64(1) {
		t.Fatalf("version 1: %v %v", v1, err)
	}
	if v4, err := s.Version(ctx, testType, rid, 4); err != nil || v4["title"] != "v4" || v4["kind"] != nil {
		t.Fatalf("version 4: %v %v", v4, err)
	}
	for _, n := range []int64{0, 5} {
		if _, err := s.Version(ctx, testType, rid, n); !errors.Is(err, ErrNotFound) {
			t.Fatalf("version %d: %v", n, err)
		}
	}

	// Restoring version 1 needs the current version and writes version 5 with its values.
	if _, err := s.RestoreVersion(ctx, testType, rid, 1, 3, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore with a stale version: %v", err)
	}
	restored, err := s.RestoreVersion(ctx, testType, rid, 1, 4, nil)
	if err != nil || restored["version"] != int64(5) || restored["title"] != "v1" || restored["kind"] != "a" || restored["priority"] != 3.0 {
		t.Fatalf("restore: %v %v", restored, err)
	}
	if cmp, err = s.Compare(ctx, testType, rid, 1, 5); err != nil || len(cmp.Changes) != 0 {
		t.Fatalf("version 5 equals version 1: %+v %v", cmp, err)
	}

	// History cut off by retention (PL39): versions before the oldest row are gone.
	if _, err := pool.Exec(ctx, "DELETE FROM audit WHERE type_name = $1 AND record_id = $2 AND version < 3", testType, rid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Version(ctx, testType, rid, 1); err == nil || !strings.Contains(err.Error(), "no longer in the history") {
		t.Fatalf("version 1 without its history: %v", err)
	}
	if v3, err := s.Version(ctx, testType, rid, 3); err != nil || v3["title"] != "v2" {
		t.Fatalf("version 3 with the rest of the history: %v %v", v3, err)
	}
}
