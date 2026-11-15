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
)

// The grants "involved" without a condition become one condition with every holder once
// (PL57): found with H14, a role assigned four times and a group named four times made four
// conditions, each of which PostgreSQL answered by hashing every actor row of the group.
func TestInvolvedGrantsBecomeOneCondition(t *testing.T) {
	person, group, other := uuid.New(), uuid.New(), uuid.New()
	g := func(role, at, cond string) Grant {
		return Grant{TypeName: "x.ticket", Action: ActionRead, Scope: ScopeInvolved, OrgPath: at, Role: role, Condition: cond}
	}
	a := &Access{AccountID: uuid.New(), PersonID: &person, Groups: []uuid.UUID{group, group, other, group},
		Grants: []Grant{g("team", "root", ""), g("team", "root", ""), g("desk", "root.a", ""), g("team", "root", "")}}
	where, args := a.Filter("x.ticket", ActionRead, nil)
	if n := strings.Count(where, "SELECT act.record_id"); n != 1 {
		t.Fatalf("%d actor conditions, want 1: %s", n, where)
	}
	if !strings.Contains(where, "org_path <@ $1::ltree OR org_path <@ $2::ltree") {
		t.Fatalf("both nodes in one condition: %s", where)
	}
	var holders []uuid.UUID
	for _, v := range args {
		if ids, ok := v.([]uuid.UUID); ok {
			holders = ids
		}
	}
	if len(holders) != 3 || holders[0] != person {
		t.Fatalf("every holder once, the person first: %v", holders)
	}
	// What the condition lets read is the same as before.
	for _, c := range []struct {
		name string
		rec  map[string]any
		want bool
	}{
		{"my group holds a role", map[string]any{"org_path": "root.b", "actors": map[string]any{"group": []any{map[string]any{"type": "core.group", "id": group.String()}}}}, true},
		{"another group of mine, below the second node", map[string]any{"org_path": "root.a.x", "actors": map[string]any{"group": []any{map[string]any{"type": "core.group", "id": other.String()}}}}, true},
		{"nobody of mine", map[string]any{"org_path": "root.a", "actors": map[string]any{"group": []any{map[string]any{"type": "core.group", "id": uuid.New().String()}}}}, false},
	} {
		if got := a.Permits("x.ticket", ActionRead, c.rec); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// A grant with a condition keeps its own.
	a.Grants = append(a.Grants, g("lead", "root", `record.state == "open"`))
	where, _ = a.Filter("x.ticket", ActionRead, nil)
	if n := strings.Count(where, "SELECT act.record_id"); n != 2 {
		t.Fatalf("%d actor conditions with a conditioned grant, want 2: %s", n, where)
	}
}
