// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

// The Prüfweg of PL74: two records and a dashboard in the trash, the list over all types shows
// three with type, title, time and person; one type shows one; a person without the right to
// read dashboards sees two; after a restore the entry is gone. Other tests leave records in
// the trash of the shared database, so every request narrows to the tag of this run.
func TestTrashOverAllTypes(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	tag := "PL74 " + uuid.NewString()[:8]
	trash := func(api *testAPI, query string) (int, []map[string]any, string) {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/trash?q="+url.QueryEscape(tag)+query, "", nil)
		if r.code != 200 {
			return r.code, nil, r.raw
		}
		var items []map[string]any
		for _, it := range r.body["items"].([]any) {
			items = append(items, it.(map[string]any))
		}
		next, _ := r.body["next"].(string)
		return r.code, items, next
	}

	var ids []string
	for _, c := range []struct{ typ, body string }{
		{"probe.probe", `{"title": "` + tag + ` eins"}`},
		{"probe.probe", `{"title": "` + tag + ` zwei"}`},
		{"core.dashboard", `{"name": "` + tag + ` Tafel"}`},
	} {
		r := call(t, admin, http.MethodPost, "/api/v1/records/"+c.typ, c.body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", c.typ, r.code, r.raw)
		}
		id := r.body["id"].(string)
		ids = append(ids, id)
		if r := call(t, admin, http.MethodDelete, "/api/v1/records/"+c.typ+"/"+id, "", nil); r.code != 204 {
			t.Fatalf("trash %s: %d %s", c.typ, r.code, r.raw)
		}
	}
	defer func() {
		for i, typ := range []string{"probe.probe", "probe.probe", "core.dashboard"} {
			call(t, admin, http.MethodDelete, "/api/v1/records/"+typ+"/"+ids[i], "", nil)
			call(t, admin, http.MethodPost, "/api/v1/records/"+typ+"/"+ids[i]+":purge", "", nil)
		}
	}()

	code, items, next := trash(admin, "")
	if code != 200 || len(items) != 3 || next != "" {
		t.Fatalf("all: %d %v %s", code, items, next)
	}
	// Newest first: the dashboard went last.
	if items[0]["type"] != "core.dashboard" || items[0]["id"] != ids[2] || items[0]["title"] != tag+" Tafel" {
		t.Fatalf("first entry: %v", items[0])
	}
	for _, e := range items {
		if e["deleted_at"] == nil || e["deleted_by"] == nil || e["deleted_by_name"] == nil || e["deleted_by_name"] == "" {
			t.Fatalf("entry without time or person: %v", e)
		}
	}
	if code, items, raw := trash(admin, "&type=core.dashboard"); len(items) != 1 || items[0]["id"] != ids[2] {
		t.Fatalf("one type: %d %v %s", code, items, raw)
	}

	// Pages by the cursor: two, then the last one.
	_, first, next := trash(admin, "&limit=2")
	if len(first) != 2 || next == "" {
		t.Fatalf("first page: %v %q", first, next)
	}
	_, rest, last := trash(admin, "&limit=2&after="+url.QueryEscape(next))
	if len(rest) != 1 || last != "" || rest[0]["id"] == first[0]["id"] || rest[0]["id"] == first[1]["id"] {
		t.Fatalf("second page: %v %q", rest, last)
	}
	if code, _, raw := trash(admin, "&after=kein-cursor"); code != 422 {
		t.Fatalf("bad cursor: %d %s", code, raw)
	}
	if code, _, raw := trash(admin, "&type=gibt.es_nicht"); code != 404 {
		t.Fatalf("unknown type: %d %s", code, raw)
	}

	// A person who reads probes but no dashboards sees the two probes only.
	const role, name = "trash_test", "apitest_trash"
	dropRole(t, admin, role)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", `{"type_name": "probe.probe", "action": "read", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}
	acc, err := admin.auth.CreateAccount(ctx, name, name, testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "`+name+`"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "`+role+`", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	person := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if _, items, _ := trash(person, ""); len(items) != 2 || items[0]["type"] != "probe.probe" || items[1]["type"] != "probe.probe" {
		t.Fatalf("without dashboards: %v", items)
	}

	// Restored, the record leaves the trash.
	if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe/"+ids[0]+":restore", "", nil); r.code != 204 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	if _, items, _ := trash(admin, ""); len(items) != 2 || items[0]["id"] == ids[0] || items[1]["id"] == ids[0] {
		t.Fatalf("after restore: %v", items)
	}
}
