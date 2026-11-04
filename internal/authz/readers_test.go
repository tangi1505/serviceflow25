// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authz

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"group24.de/itsm/internal/meta"
)

// Readers (SD16a): a reader without roles reaches the records at the person's node or above
// that its condition holds for; one with roles only their holders, wherever they are; nobody
// writes by a reader, and the tenant bounds both.
func TestReaders(t *testing.T) {
	person, group, other := uuid.New(), uuid.New(), uuid.New()
	rs := []meta.Reader{
		{When: `record.state == "published" && record.audience == "all"`},
		{When: `record.state == "published" && record.audience == "groups"`, Roles: []string{"shared"}},
	}
	a := (&Access{AccountID: uuid.New(), PersonID: &person, Groups: []uuid.UUID{group}, PersonNode: "root.a.b",
		Tenants: []string{"root.a"}, Readers: map[string][]meta.Reader{"k.article": rs}}).Enter("root.a")
	holder := func(role string, id uuid.UUID) map[string]any {
		return map[string]any{role: []any{map[string]any{"type": "core.group", "id": id.String()}}}
	}
	for _, c := range []struct {
		name string
		rec  map[string]any
		want bool
	}{
		{"for all at a node above", map[string]any{"org_path": "root.a", "state": "published", "audience": "all"}, true},
		{"for all at the own node", map[string]any{"org_path": "root.a.b", "state": "published", "audience": "all"}, true},
		{"a draft", map[string]any{"org_path": "root.a", "state": "draft", "audience": "all"}, false},
		{"only for agents", map[string]any{"org_path": "root.a", "state": "published", "audience": "agents"}, false},
		{"for all at a node beside", map[string]any{"org_path": "root.a.c", "state": "published", "audience": "all"}, false},
		{"for all in another tenant", map[string]any{"org_path": "root.x", "state": "published", "audience": "all"}, false},
		{"for my group anywhere", map[string]any{"org_path": "root.a.c", "state": "published", "audience": "groups", "actors": holder("shared", group)}, true},
		{"for another group", map[string]any{"org_path": "root.a.c", "state": "published", "audience": "groups", "actors": holder("shared", other)}, false},
		{"my group in another role", map[string]any{"org_path": "root.a", "state": "published", "audience": "groups", "actors": holder("author", group)}, false},
	} {
		if got := a.Permits("k.article", ActionRead, c.rec); got != c.want {
			t.Errorf("%s: read = %v, want %v", c.name, got, c.want)
		}
	}
	if a.Permits("k.article", ActionUpdate, map[string]any{"org_path": "root.a", "state": "published", "audience": "all"}) {
		t.Error("a reader may update")
	}
	if !a.Allowed("k.article", ActionRead) || a.Allowed("k.article", ActionCreate) {
		t.Error("Allowed: a reader reads and creates nothing")
	}
	where, args := a.Filter("k.article", ActionRead, nil)
	if !strings.Contains(where, "act.role = ANY") || !strings.Contains(where, "<@ org_path") || len(args) == 0 {
		t.Errorf("filter of the readers: %s %v", where, args)
	}
	if where, _ := a.Filter("k.article", ActionUpdate, nil); where != "FALSE" {
		t.Errorf("filter for update = %s, want FALSE", where)
	}
	// A condition that does not translate to SQL lets nobody read, in the filter and in Go.
	bad := (&Access{AccountID: uuid.New(), PersonNode: "root", Tenants: []string{"root"},
		Readers: map[string][]meta.Reader{"k.article": {{When: `record.title.matches("^a")`}}}})
	if where, _ := bad.Filter("k.article", ActionRead, nil); where != "FALSE" {
		t.Errorf("untranslatable filter = %s, want FALSE", where)
	}
	if bad.Permits("k.article", ActionRead, map[string]any{"org_path": "root", "title": "abc"}) {
		t.Error("an untranslatable reader lets read")
	}
}
