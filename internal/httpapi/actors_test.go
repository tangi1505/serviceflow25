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
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The Prüfweg of WF3: a record with an assigned group is found by every member of the group
// under "my groups" (and only by them, through the scope involved); a role of another type
// cannot be chosen; the roles are part of the version, the history and the type's OpenAPI.
func TestRolesAtTheRecordOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")

	// The role wf3_involved reads and changes probe.probe where one holds a role.
	call(t, admin, http.MethodDelete, "/api/v1/roles/wf3_involved", "", nil)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "wf3_involved", "label": {"de": "Beteiligt", "en": "Involved"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, action := range []string{"read", "update"} {
		body := `{"type_name": "probe.probe", "action": "` + action + `", "scope": "involved"}`
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/wf3_involved/permissions", body, nil); r.code != 204 {
			t.Fatalf("permission %s: %d %s", action, r.code, r.raw)
		}
	}

	// Anna is in the group Netz, Ben is not; both hold the role.
	group := call(t, admin, http.MethodPost, "/api/v1/records/core.group", `{"name": "WF3 Netz `+suffix+`"}`, nil)
	if group.code != 201 {
		t.Fatalf("group: %d %s", group.code, group.raw)
	}
	gid := group.body["id"].(string)
	type user struct {
		api    *testAPI
		person string
	}
	login := func(name string) user {
		t.Helper()
		acc, err := admin.auth.CreateAccount(ctx, name, name, testPassword, nil)
		if err != nil {
			t.Fatal(err)
		}
		p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "`+name+`"}`, nil)
		if p.code != 201 {
			t.Fatalf("person: %d %s", p.code, p.raw)
		}
		pid := uuid.MustParse(p.body["id"].(string))
		if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
			t.Fatal(err)
		}
		if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "wf3_involved", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
			t.Fatalf("assign: %d %s", r.code, r.raw)
		}
		in := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
		if in.code != 200 {
			t.Fatalf("login %s: %d %s", name, in.code, in.raw)
		}
		return user{api: &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, in.hdr)}, person: pid.String()}
	}
	anna, ben := login("apitest_wf3_anna"), login("apitest_wf3_ben")
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+gid+`", "person": "`+anna.person+`"}`, nil); r.code != 201 {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}

	// A record with the group as assigned and Ben as requester.
	rec := call(t, admin, http.MethodPost, base, `{"title": "WF3 Router `+suffix+`", "actors": {
		"assigned": [{"type": "core.group", "id": "`+gid+`"}],
		"requester": [{"type": "core.person", "id": "`+ben.person+`"}]}}`, nil)
	if rec.code != 201 {
		t.Fatalf("create with roles: %d %s", rec.code, rec.raw)
	}
	id := rec.body["id"].(string)
	actors, _ := rec.body["actors"].(map[string]any)
	if assigned, _ := actors["assigned"].([]any); len(assigned) != 1 || actors["observer"] == nil {
		t.Fatalf("roles in the answer: %s", rec.raw)
	}

	// Every member finds it under "my groups"; Ben finds it as requester, not under his groups.
	mine := func(u user, filter string) []string {
		t.Helper()
		r := call(t, u.api, http.MethodGet, base+"?limit=100&filter[actors.assigned]="+filter, "", nil)
		if r.code != 200 {
			t.Fatalf("list: %d %s", r.code, r.raw)
		}
		var page struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal([]byte(r.raw), &page)
		out := []string{}
		for _, it := range page.Items {
			out = append(out, it["id"].(string))
		}
		return out
	}
	if got := mine(anna, "@my_groups"); len(got) != 1 || got[0] != id {
		t.Fatalf("anna under my groups: %v", got)
	}
	if got := mine(ben, "@my_groups"); len(got) != 0 {
		t.Fatalf("ben under my groups: %v", got)
	}
	if r := call(t, ben.api, http.MethodGet, base+"/"+id, "", nil); r.code != 200 {
		t.Fatalf("ben reads it as requester: %d %s", r.code, r.raw)
	}
	q := call(t, anna.api, http.MethodPost, base+":query", `{"where": {"field": "actors.assigned", "eq": "@my_groups"}, "fields": ["title", "actors"]}`, nil)
	if q.code != 200 || !strings.Contains(q.raw, id) {
		t.Fatalf("query my groups: %d %s", q.code, q.raw)
	}

	// Ben is no longer requester: the record is gone for him (404), anna still has it.
	upd := call(t, anna.api, http.MethodPatch, base+"/"+id, `{"actors": {"requester": null}}`, map[string]string{"If-Match": `"1"`})
	if upd.code != 200 || upd.body["version"] != float64(2) {
		t.Fatalf("clear requester: %d %s", upd.code, upd.raw)
	}
	if r := call(t, ben.api, http.MethodGet, base+"/"+id, "", nil); r.code != 404 {
		t.Fatalf("ben after losing the role: %d %s", r.code, r.raw)
	}

	// A role of another type, or a holder type the role does not take, cannot be chosen.
	bad := call(t, admin, http.MethodPatch, base+"/"+id, `{"actors": {"vendor": [{"type": "core.group", "id": "`+gid+`"}], "observer": [{"type": "probe.color", "id": "`+gid+`"}]}}`, map[string]string{"If-Match": `"2"`})
	if bad.code != 422 || !strings.Contains(bad.raw, `"actors.vendor"`) || !strings.Contains(bad.raw, `"actors.observer"`) {
		t.Fatalf("roles of another type: %d %s", bad.code, bad.raw)
	}

	// The history names the role; the type and its OpenAPI show the roles.
	h := call(t, admin, http.MethodGet, base+"/"+id+"/history", "", nil)
	if h.code != 200 || !strings.Contains(h.raw, `actors.requester`) || !strings.Contains(h.raw, `actors.assigned`) {
		t.Fatalf("history: %d %s", h.code, h.raw)
	}
	ty := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
	if ty.code != 200 || !strings.Contains(ty.raw, `"name":"assigned"`) || !strings.Contains(ty.raw, `"to":["core.person","core.group"]`) {
		t.Fatalf("type: %d %s", ty.code, ty.raw)
	}
	spec := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	if spec.code != 200 || !strings.Contains(spec.raw, `"actors"`) || !strings.Contains(spec.raw, `"involved"`) {
		t.Fatalf("openapi: %d", spec.code)
	}

	// Clean up the record (with its actor rows) and the group.
	for _, step := range []struct{ method, path string }{
		{http.MethodDelete, base + "/" + id}, {http.MethodPost, base + "/" + id + ":purge"},
		{http.MethodDelete, "/api/v1/roles/wf3_involved"},
	} {
		if r := call(t, admin, step.method, step.path, "", nil); r.code != 204 {
			t.Fatalf("%s %s: %d %s", step.method, step.path, r.code, r.raw)
		}
	}
}
