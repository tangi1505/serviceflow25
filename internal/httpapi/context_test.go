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

// The Prüfweg of PL10: a person with roles at two nodes narrows the session to one of them;
// the list then shows only the records of that unit, and the narrowing shows in auth/me.
func TestSessionContextNarrowsTheView(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"

	for _, path := range []string{"root.ctx.east", "root.ctx.west", "root.ctx"} {
		call(t, admin, http.MethodDelete, "/api/v1/org/nodes/"+path, "", nil)
	}
	for _, body := range []string{`{"parent": "root", "name": "ctx"}`, `{"parent": "root.ctx", "name": "east"}`, `{"parent": "root.ctx", "name": "west"}`} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", body, nil); r.code != 201 {
			t.Fatalf("node: %d %s", r.code, r.raw)
		}
	}
	call(t, admin, http.MethodDelete, "/api/v1/roles/ctx_reader", "", nil)
	call(t, admin, http.MethodDelete, "/api/v1/roles/ctx_writer", "", nil)
	for _, role := range []string{"ctx_reader", "ctx_writer"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`"}`, nil); r.code != 201 {
			t.Fatalf("role: %d %s", r.code, r.raw)
		}
	}
	for _, action := range []string{"read", "create"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/ctx_reader/permissions", `{"type_name": "probe.probe", "action": "`+action+`", "scope": "subtree"}`, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/ctx_writer/permissions", `{"type_name": "probe.probe", "action": "update", "scope": "subtree"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}

	acc, err := admin.auth.CreateAccount(ctx, "apitest_ctx", "apitest_ctx", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "apitest_ctx"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	// Reader at east and west, writer only at east.
	for _, a := range []string{
		`{"role": "ctx_reader", "person_id": "` + pid.String() + `", "org_path": "root.ctx.east"}`,
		`{"role": "ctx_reader", "person_id": "` + pid.String() + `", "org_path": "root.ctx.west"}`,
		`{"role": "ctx_writer", "person_id": "` + pid.String() + `", "org_path": "root.ctx.east"}`,
	} {
		if r := call(t, admin, http.MethodPost, "/api/v1/assignments", a, nil); r.code != 201 {
			t.Fatalf("assign: %d %s", r.code, r.raw)
		}
	}
	east := call(t, admin, http.MethodPost, base, `{"title": "ctx east", "org_path": "root.ctx.east"}`, nil)
	west := call(t, admin, http.MethodPost, base, `{"title": "ctx west", "org_path": "root.ctx.west"}`, nil)
	if east.code != 201 || west.code != 201 {
		t.Fatalf("records: %d %d", east.code, west.code)
	}

	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_ctx", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}

	titles := func() []string {
		t.Helper()
		r := call(t, user, http.MethodGet, base+"?filter[org_path]=root.ctx", "", nil)
		if r.code != 200 {
			t.Fatalf("list: %d %s", r.code, r.raw)
		}
		var page struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal([]byte(r.raw), &page)
		var out []string
		for _, it := range page.Items {
			out = append(out, it["title"].(string))
		}
		return out
	}
	if got := titles(); len(got) != 2 {
		t.Fatalf("whole view: %v", got)
	}
	me := call(t, user, http.MethodGet, "/api/v1/auth/me", "", nil)
	if me.code != 200 || me.body["active_org"] != nil || len(me.body["roles"].([]any)) != 2 || len(me.body["nodes"].([]any)) != 2 {
		t.Fatalf("me before: %s", me.raw)
	}

	// Narrow to west: only the west record; create lands there; update is refused there.
	r := call(t, user, http.MethodPost, "/api/v1/auth/context", `{"org_path": "root.ctx.west"}`, nil)
	if r.code != 200 || r.body["active_org"] != "root.ctx.west" {
		t.Fatalf("set context: %d %s", r.code, r.raw)
	}
	if got := titles(); len(got) != 1 || got[0] != "ctx west" {
		t.Fatalf("west view: %v", got)
	}
	if r := call(t, user, http.MethodGet, base+"/"+east.body["id"].(string), "", nil); r.code != 404 {
		t.Fatalf("east while narrowed to west: %d", r.code)
	}
	if r := call(t, user, http.MethodPost, base, `{"title": "ctx new east", "org_path": "root.ctx.east"}`, nil); r.code != 403 {
		t.Fatalf("create in east while narrowed to west: %d %s", r.code, r.raw)
	}
	me = call(t, user, http.MethodGet, "/api/v1/auth/me", "", nil)
	if me.body["active_org"] != "root.ctx.west" {
		t.Fatalf("me after: %s", me.raw)
	}

	// Narrow to the reader role only: update at east is refused although the writer role exists.
	if r := call(t, user, http.MethodPost, "/api/v1/auth/context", `{"role": "ctx_reader", "org_path": ""}`, nil); r.code != 200 || r.body["active_role"] != "ctx_reader" {
		t.Fatalf("set role: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPatch, base+"/"+east.body["id"].(string), `{"title": "x"}`, map[string]string{"If-Match": `"1"`}); r.code != 403 {
		t.Fatalf("update with reader role only: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPost, "/api/v1/auth/context", `{}`, nil); r.code != 200 {
		t.Fatalf("lift: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPatch, base+"/"+east.body["id"].(string), `{"title": "ctx east 2"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("update with every role: %d %s", r.code, r.raw)
	}

	// Choices the person does not hold are refused.
	if r := call(t, user, http.MethodPost, "/api/v1/auth/context", `{"role": "admin"}`, nil); r.code != 422 {
		t.Fatalf("foreign role: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPost, "/api/v1/auth/context", `{"org_path": "root"}`, nil); r.code != 422 {
		t.Fatalf("node above the roles: %d %s", r.code, r.raw)
	}
}
