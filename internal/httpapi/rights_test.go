// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// The Prüfweg of PL8: three persons with another scope each (own, unit, subtree) see exactly
// their records in list, single read and count; a fourth reaches records through a group; an
// action without a grant is 403, an invisible record 404; managing roles needs the right.
func TestRightsScopesOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"

	// Nodes root.rights and root.rights.sub (leftovers of earlier runs removed).
	call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.rights.sub", "", nil)
	call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.rights", "", nil)
	for _, body := range []string{`{"parent": "root", "name": "rights"}`, `{"parent": "root.rights", "name": "sub"}`} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", body, nil); r.code != 201 {
			t.Fatalf("node: %d %s", r.code, r.raw)
		}
	}

	// Roles: one per scope on probe.probe, read and create; no update.
	for _, role := range []string{"rights_own", "rights_unit", "rights_subtree", "rights_group"} {
		dropRole(t, admin, role)
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "`+role+`"}}`, nil); r.code != 201 {
			t.Fatalf("role %s: %d %s", role, r.code, r.raw)
		}
		scope := map[string]string{"rights_own": "own", "rights_unit": "unit", "rights_subtree": "subtree", "rights_group": "group"}[role]
		for _, action := range []string{"read", "create"} {
			body := `{"type_name": "probe.probe", "action": "` + action + `", "scope": "` + scope + `"}`
			if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", body, nil); r.code != 204 {
				t.Fatalf("permission %s %s: %d %s", role, action, r.code, r.raw)
			}
		}
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/rights_own/permissions", `{"type_name": "probe.probe", "action": "read", "scope": "galaxy"}`, nil); r.code != 422 {
		t.Fatalf("bad scope: %d %s", r.code, r.raw)
	}
	r := call(t, admin, http.MethodGet, "/api/v1/roles/rights_own", "", nil)
	if r.code != 200 {
		t.Fatalf("get role: %d %s", r.code, r.raw)
	}
	var role struct {
		Permissions []map[string]any `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(r.raw), &role); err != nil || len(role.Permissions) != 2 {
		t.Fatalf("role permissions: %s", r.raw)
	}

	// Four persons with accounts; each gets a role at a node.
	type user struct {
		name   string
		role   string
		node   string
		api    *testAPI
		person string
	}
	users := []*user{
		{name: "apitest_own", role: "rights_own", node: "root.rights"},
		{name: "apitest_unit", role: "rights_unit", node: "root.rights"},
		{name: "apitest_subtree", role: "rights_subtree", node: "root.rights"},
		{name: "apitest_mate", role: "rights_group", node: "root.rights"},
	}
	for _, u := range users {
		acc, err := admin.auth.CreateAccount(ctx, u.name, u.name, testPassword, nil)
		if err != nil {
			t.Fatal(err)
		}
		p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "`+u.name+`"}`, nil)
		if p.code != 201 {
			t.Fatalf("person: %d %s", p.code, p.raw)
		}
		u.person = p.body["id"].(string)
		pid := uuid.MustParse(u.person)
		if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
			t.Fatal(err)
		}
		body := `{"role": "` + u.role + `", "person_id": "` + u.person + `", "org_path": "` + u.node + `"}`
		if r := call(t, admin, http.MethodPost, "/api/v1/assignments", body, nil); r.code != 201 {
			t.Fatalf("assign %s: %d %s", u.name, r.code, r.raw)
		}
		login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+u.name+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
		if login.code != 200 {
			t.Fatalf("login %s: %d %s", u.name, login.code, login.raw)
		}
		u.api = &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	}
	own, unit, subtree, mate := users[0], users[1], users[2], users[3]

	// mate shares a group with own: the scope "group" reaches own's records.
	g := call(t, admin, http.MethodPost, "/api/v1/records/core.group", `{"name": "rights mates"}`, nil)
	for _, u := range []*user{own, mate} {
		if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+g.body["id"].(string)+`", "person": "`+u.person+`"}`, nil); r.code != 201 {
			t.Fatalf("member: %d %s", r.code, r.raw)
		}
	}

	// Records: by admin at root.rights (r1) and root.rights.sub (r2) and root (r0); by own at
	// root.rights (r3).
	create := func(api *testAPI, body string) resp {
		t.Helper()
		return call(t, api, http.MethodPost, base, body, nil)
	}
	r0 := create(admin, `{"title": "rights r0"}`)
	r1 := create(admin, `{"title": "rights r1", "org_path": "root.rights"}`)
	r2 := create(admin, `{"title": "rights r2", "org_path": "root.rights.sub"}`)
	r3 := create(own.api, `{"title": "rights r3", "org_path": "root.rights"}`)
	for _, r := range []resp{r0, r1, r2, r3} {
		if r.code != 201 {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
	}
	// Naming the root, outside the node: refused. Without a node the record lands in the one
	// node of the right since PL46 (checked at the end, after the counts).
	if r := create(own.api, `{"title": "rights r4", "org_path": "root"}`); r.code != 403 {
		t.Fatalf("own creating at root (outside the node): %d %s", r.code, r.raw)
	}
	if r := create(unit.api, `{"title": "rights r5", "org_path": "root.rights.sub"}`); r.code != 403 {
		t.Fatalf("unit creating below the node: %d %s", r.code, r.raw)
	}

	ids := func(api *testAPI) (map[string]bool, int64) {
		t.Helper()
		r := call(t, api, http.MethodGet, base+"?count=true&limit=100", "", nil)
		if r.code != 200 {
			t.Fatalf("list: %d %s", r.code, r.raw)
		}
		var page struct {
			Items []map[string]any `json:"items"`
			Total *int64           `json:"total"`
		}
		if err := json.Unmarshal([]byte(r.raw), &page); err != nil || page.Total == nil {
			t.Fatalf("page: %v %s", err, r.raw)
		}
		out := map[string]bool{}
		for _, it := range page.Items {
			out[it["id"].(string)] = true
		}
		return out, *page.Total
	}
	expect := func(u *user, visible ...resp) {
		t.Helper()
		seen, total := ids(u.api)
		want := map[string]bool{}
		for _, v := range visible {
			want[v.body["id"].(string)] = true
		}
		for _, v := range []resp{r0, r1, r2, r3} {
			id := v.body["id"].(string)
			if seen[id] != want[id] {
				t.Fatalf("%s sees %s = %v, want %v", u.name, v.body["title"], seen[id], want[id])
			}
			r := call(t, u.api, http.MethodGet, base+"/"+id, "", nil)
			if (r.code == 200) != want[id] {
				t.Fatalf("%s reads %s: %d, want visible %v", u.name, v.body["title"], r.code, want[id])
			}
		}
		if total != int64(len(seen)) {
			t.Fatalf("%s count %d, list %d", u.name, total, len(seen))
		}
	}
	expect(own, r3)
	expect(unit, r1, r3)
	expect(subtree, r1, r2, r3)
	expect(mate, r3)
	if seen, _ := ids(admin); len(seen) < 4 {
		t.Fatalf("admin sees %d", len(seen))
	}

	// No update right: visible but forbidden; invisible: not found. History follows read.
	id3 := r3.body["id"].(string)
	if r := call(t, unit.api, http.MethodPatch, base+"/"+id3, `{"title": "x"}`, map[string]string{"If-Match": `"1"`}); r.code != 403 || r.body["type"] != "/problems/forbidden" {
		t.Fatalf("update without right: %d %s", r.code, r.raw)
	}
	if r := call(t, own.api, http.MethodPatch, base+"/"+r1.body["id"].(string), `{"title": "x"}`, map[string]string{"If-Match": `"1"`}); r.code != 404 {
		t.Fatalf("update invisible: %d %s", r.code, r.raw)
	}
	if r := call(t, own.api, http.MethodDelete, base+"/"+id3, "", nil); r.code != 403 {
		t.Fatalf("trash without right: %d %s", r.code, r.raw)
	}
	if r := call(t, own.api, http.MethodGet, base+"/"+r1.body["id"].(string)+"/history", "", nil); r.code != 404 {
		t.Fatalf("history of invisible: %d", r.code)
	}
	if r := call(t, own.api, http.MethodPost, base+"/"+id3+":transition", `{"transition": "start"}`, nil); r.code != 403 {
		t.Fatalf("transition without update right: %d %s", r.code, r.raw)
	}

	// Managing roles needs manage_rights; reading them does not.
	if r := call(t, own.api, http.MethodGet, "/api/v1/roles", "", nil); r.code != 200 {
		t.Fatalf("roles for own: %d", r.code)
	}
	if r := call(t, own.api, http.MethodPost, "/api/v1/roles", `{"name": "rights_sneaky"}`, nil); r.code != 403 {
		t.Fatalf("own creating a role: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/admin", "", nil); r.code != 422 {
		t.Fatalf("deleting admin: %d %s", r.code, r.raw)
	}

	// Revoking the assignment takes effect on the next request.
	r = call(t, admin, http.MethodGet, "/api/v1/assignments", "", nil)
	var list []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &list)
	for _, a := range list {
		if a["person_id"] == subtree.person {
			if r := call(t, admin, http.MethodDelete, "/api/v1/assignments/"+a["id"].(string), "", nil); r.code != 204 {
				t.Fatalf("unassign: %d %s", r.code, r.raw)
			}
		}
	}
	if r := call(t, subtree.api, http.MethodGet, base+"/"+id3, "", nil); r.code != 404 {
		t.Fatalf("after revoke: %d", r.code)
	}
	if seen, total := ids(subtree.api); len(seen) != 0 || total != 0 {
		t.Fatalf("after revoke list: %d %d", len(seen), total)
	}

	// Without a node a record lands in the one node of the right (PL46); before, at the root
	// and so 403.
	if r := create(own.api, `{"title": "rights r6"}`); r.code != 201 || r.body["org_path"] != "root.rights" {
		t.Fatalf("own creating without a node: %d %s", r.code, r.raw)
	}
}
