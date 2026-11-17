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

// A grant without condition over all records, or over a node with all below, covers the grants
// within it (PL70): found with H14, the load account read all incidents and was "involved"
// beside, so it counted 18 million rows instead of reading the day values.
func TestCoveredGrants(t *testing.T) {
	person := uuid.New()
	g := func(scope Scope, at, cond string) Grant {
		return Grant{TypeName: "x.ticket", Action: ActionRead, Scope: scope, OrgPath: at, Role: "r", Condition: cond}
	}
	access := func(gs ...Grant) *Access {
		return &Access{AccountID: uuid.New(), PersonID: &person, Grants: gs,
			Readers: map[string][]meta.Reader{"x.ticket": {{Roles: []string{"caller"}}}}}
	}
	for _, c := range []struct {
		name   string
		a      *Access
		where  string
		absent string
		units  bool
	}{
		{"all and involved", access(g(ScopeAll, "root", ""), g(ScopeInvolved, "root", "")), "(TRUE)", "", true},
		{"involved first, then all", access(g(ScopeInvolved, "root.a", ""), g(ScopeAll, "root", "")), "(TRUE)", "", true},
		{"subtree and own below it", access(g(ScopeSubtree, "root.a", ""), g(ScopeOwn, "root.a.b", ""), g(ScopeUnit, "root.a", "")), "", "created_by", false},
		{"subtree and involved beside it", access(g(ScopeSubtree, "root.a", ""), g(ScopeInvolved, "root.ab", "")), "", "", false},
		{"all with a condition covers nothing", access(g(ScopeAll, "root", `record.state == "open"`), g(ScopeInvolved, "root", "")), "", "", false},
		{"twice the same subtree", access(g(ScopeSubtree, "root.a", ""), g(ScopeSubtree, "root.a", "")), "", "", false},
	} {
		where, _ := c.a.Filter("x.ticket", ActionRead, nil)
		if c.where != "" && where != c.where {
			t.Errorf("%s: %s, want %s", c.name, where, c.where)
		}
		if c.absent != "" && strings.Contains(where, c.absent) {
			t.Errorf("%s: %q still in %s", c.name, c.absent, where)
		}
		// The readers count unless all records are read anyway.
		if c.units && strings.Contains(where, "act.role") {
			t.Errorf("%s: readers beside all records: %s", c.name, where)
		}
		if got := c.a.UnitsOnly("x.ticket"); got != c.units {
			t.Errorf("%s: UnitsOnly %v, want %v", c.name, got, c.units)
		}
	}
	// Without the readers the subtree that covers own and unit names org_path only.
	a := access(g(ScopeSubtree, "root.a", ""), g(ScopeOwn, "root.a.b", ""), g(ScopeUnit, "root.a", ""))
	a.Readers = nil
	if !a.UnitsOnly("x.ticket") {
		t.Fatal("subtree with own below it: not units only")
	}
	if where, args := a.Filter("x.ticket", ActionRead, nil); where != "(org_path <@ $1::ltree)" || len(args) != 1 {
		t.Fatalf("subtree with own below it: %s %v", where, args)
	}
	a.Grants = append(a.Grants, a.Grants[0])
	if where, _ := a.Filter("x.ticket", ActionRead, nil); strings.Count(where, "org_path <@") != 1 {
		t.Fatalf("twice the same subtree: %s", where)
	}
	// An active node keeps the readers: they reach beyond it.
	a = access(g(ScopeAll, "root", ""))
	a.ActiveOrg = "root.a"
	if where, _ := a.Filter("x.ticket", ActionRead, nil); !strings.Contains(where, "act.role") {
		t.Fatalf("active node without the readers: %s", where)
	}
}
