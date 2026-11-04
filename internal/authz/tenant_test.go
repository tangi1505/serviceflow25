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

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// The tenant bounds every scope, "all" included; a role in another tenant does not count; a
// role at the root counts in every tenant (PL44).
func TestTenantBoundary(t *testing.T) {
	admin := &Access{AccountID: uuid.New(), Tenants: []string{"root.a", "root.b"}, Grants: []Grant{
		{TypeName: Any, Action: Any, Scope: ScopeAll, OrgPath: "root", Role: "admin"},
	}}
	inA := admin.Enter("root.a")
	if inA.Tenant != "root.a" || admin.Tenant != "" {
		t.Fatalf("enter: %q, the cached access changed: %q", inA.Tenant, admin.Tenant)
	}
	if got := admin.Enter("root.c").Tenant; got != "root.a" {
		t.Fatalf("a tenant the account may not enter: %q", got)
	}
	filter, args := inA.Filter("p.t", ActionRead, nil)
	if filter != "((TRUE) AND org_path <@ $1::ltree)" || len(args) != 1 || args[0] != "root.a" {
		t.Fatalf("filter: %s %v", filter, args)
	}
	for path, want := range map[string]bool{"root.a": true, "root.a.x": true, "root.b.x": false, "root": false, "root.ab": false} {
		if got := inA.Permits("p.t", ActionUpdate, map[string]any{"org_path": path}); got != want {
			t.Errorf("permits %s: %v", path, got)
		}
	}
	if !inA.Installation("core.role", "manage_rights") {
		t.Fatal("a role at the root holds the installation")
	}

	// A role with the scope all at a node of tenant b reaches nothing in a.
	other := &Access{AccountID: uuid.New(), Tenants: []string{"root.a", "root.b"}, Grants: []Grant{
		{TypeName: Any, Action: Any, Scope: ScopeAll, OrgPath: "root.b", Role: "admin"},
		{TypeName: "p.t", Action: ActionRead, Scope: ScopeSubtree, OrgPath: "root.a.x", Role: "reader"},
	}}
	o := other.Enter("root.a")
	if o.Allowed("p.t", ActionUpdate) || o.Permits("p.t", ActionRead, map[string]any{"org_path": "root.a.y"}) {
		t.Fatal("a role of tenant b reaches into a")
	}
	if !o.Permits("p.t", ActionRead, map[string]any{"org_path": "root.a.x.z"}) {
		t.Fatal("the role in a is lost")
	}
	if roles := o.Roles(); len(roles) != 1 || roles[0] != "reader" {
		t.Fatalf("roles in a: %v", roles)
	}
	if o.Installation("core.role", "manage_rights") || other.Enter("root.b").Installation("core.role", "manage_rights") {
		t.Fatal("a role at a tenant holds the installation")
	}

	// Select lists: the tenant's own copy only (PL45, V28), and of it the entries at or above
	// the node one sees them from, here the tenant (SD-Plan N5; the filter has the node since).
	// The rule of the list stands in parentheses since the readers of SD16a join with OR.
	lists := &Access{AccountID: uuid.New(), Tenants: []string{"root.a"}, Readers: map[string][]meta.Reader{"core.wait_reason": {{}}}}
	filter, args = lists.Filter("core.wait_reason", ActionRead, nil)
	if filter != "(($1::ltree <@ org_path) AND org_path <@ $2::ltree)" || args[0] != "root.a" || args[1] != "root.a" {
		t.Fatalf("list filter: %s %v", filter, args)
	}
	if !lists.Permits("core.wait_reason", ActionRead, map[string]any{"org_path": "root.a"}) ||
		lists.Permits("core.wait_reason", ActionRead, map[string]any{"org_path": "root"}) ||
		lists.Permits("core.wait_reason", ActionRead, map[string]any{"org_path": "root.b"}) {
		t.Fatal("list rights")
	}
	// The system inside one tenant: every right, only there.
	in := SystemIn("root.a")
	if in.IsSystem() || !in.Permits("p.t", ActionUpdate, map[string]any{"org_path": "root.a.x"}) ||
		in.Permits("p.t", ActionRead, map[string]any{"org_path": "root.b"}) || !SystemIn("root").IsSystem() {
		t.Fatal("SystemIn")
	}

	// Work on behalf of an account outside a request (jobs) stays in its first tenant; without
	// tenants (the root) and for the system there is no boundary.
	if f, _ := admin.Filter("p.t", ActionRead, nil); !strings.Contains(f, "org_path <@") {
		t.Fatalf("no request, no boundary: %s", f)
	}
	single := &Access{AccountID: uuid.New(), Tenants: []string{"root"}, Grants: admin.Grants}
	if f, _ := single.Filter("p.t", ActionRead, nil); f != "(TRUE)" || single.HomeNode() != "root" {
		t.Fatalf("one tenant: %s", f)
	}
	if f, _ := System().Filter("p.t", ActionRead, nil); f != "TRUE" {
		t.Fatalf("system: %s", f)
	}
	none := &Access{AccountID: uuid.New(), Tenants: []string{NoTenant}, Readers: map[string][]meta.Reader{"core.wait_reason": {{}}}}
	if none.Permits("core.wait_reason", ActionRead, map[string]any{"org_path": "root.a"}) || none.HomeNode() != "root" {
		t.Fatal("an account without a tenant reaches into one")
	}
}

// Where a new record lands that names no node (PL46): the node of the context, else the
// person's node if the access may create there, else the one node with the right; two nodes
// leave a choice. The reach lists the nodes to choose from, inner ones swallowed.
func TestCreateNode(t *testing.T) {
	g := func(path string, scope Scope) Grant {
		return Grant{TypeName: Any, Action: ActionCreate, Scope: scope, OrgPath: path, Role: "r"}
	}
	root := []string{"root"}
	for _, c := range []struct {
		name  string
		a     Access
		node  string
		ok    bool
		reach string
	}{
		{"one subtree", Access{Tenants: root, Grants: []Grant{g("root.a", ScopeSubtree)}}, "root.a", true, "root.a+"},
		{"person inside", Access{Tenants: root, PersonNode: "root.a.x", Grants: []Grant{g("root.a", ScopeSubtree)}}, "root.a.x", true, "root.a+"},
		{"person outside", Access{Tenants: root, PersonNode: "root.b", Grants: []Grant{g("root.a", ScopeSubtree)}}, "root.a", true, "root.a+"},
		{"two nodes", Access{Tenants: root, Grants: []Grant{g("root.a", ScopeSubtree), g("root.b", ScopeUnit)}}, "", false, "root.a+,root.b"},
		{"inner swallowed", Access{Tenants: root, Grants: []Grant{g("root.a", ScopeAll), g("root.a.x", ScopeUnit)}}, "root.a", true, "root.a+"},
		{"context", Access{Tenants: root, ActiveOrg: "root.a.y", Grants: []Grant{g("root.a", ScopeSubtree), g("root.b", ScopeSubtree)}}, "root.a.y", true, "root.a.y+"},
		{"role at the root, in a tenant", Access{Tenants: []string{"root.a", "root.b"}, Grants: []Grant{g("root", ScopeAll)}}, "root.a", true, "root.a+"},
		{"no right", Access{Tenants: []string{"root.a"}}, "root.a", true, ""},
	} {
		node, ok := c.a.CreateNode("p.t")
		var parts []string
		for _, r := range c.a.CreateReach("p.t") {
			s := r.Path
			if r.Below {
				s += "+"
			}
			parts = append(parts, s)
		}
		if reach := strings.Join(parts, ","); node != c.node || ok != c.ok || reach != c.reach {
			t.Errorf("%s: node %q %v, reach %s; want %q %v, %s", c.name, node, ok, reach, c.node, c.ok, c.reach)
		}
	}
}

// The tenants an account may work in.
func TestEnterable(t *testing.T) {
	tenants := []queries.ListTenantsRow{{Path: "root.a"}, {Path: "root.b"}}
	for _, c := range []struct {
		name    string
		access  Access
		tenants []queries.ListTenantsRow
		want    string
	}{
		{"no tenants", Access{}, nil, "root"},
		{"bound", Access{Home: "root.b"}, tenants, "root.b"},
		{"role at the root", Access{Grants: []Grant{{OrgPath: "root"}}}, tenants, "root.a,root.b"},
		{"role below a tenant", Access{Grants: []Grant{{OrgPath: "root.b.x"}}}, tenants, "root.b"},
		{"no role", Access{}, tenants, NoTenant},
	} {
		if got := strings.Join(enterable(&c.access, c.tenants), ","); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
