// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// Select lists over HTTP (PL16): the probe package ships two lists with entries; every
// signed-in person reads them without a role, writing needs one; a reference field refuses
// an entry of another list; the tree answers full names and subtrees.
func TestSelectListsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	// Start from the shipped entries: earlier runs moved and added some.
	if _, err := admin.pool.Exec(ctx, "DELETE FROM list_entry WHERE list LIKE 'probe.%'"); err != nil {
		t.Fatal(err)
	}
	probe, err := meta.Load("../../packages/probe")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := records.New(db.Single(admin.pool)).SeedLists(ctx, probe); err != nil || n != 6 {
		t.Fatalf("seed: %d %v", n, err)
	}

	// The type listing shows the lists with their trait and the fields of the trait.
	r := call(t, admin, http.MethodGet, "/api/v1/types/probe.place", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"traits":["list","history"]`) || !strings.Contains(r.raw, `"tree":true`) || !strings.Contains(r.raw, `"name":"parent"`) {
		t.Fatalf("place type: %d %s", r.code, r.raw)
	}
	entries := func(api *testAPI, list, query string) []map[string]any {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/records/"+list+query, "", nil)
		if r.code != 200 {
			t.Fatalf("list %s: %d %s", list, r.code, r.raw)
		}
		items, _ := r.body["items"].([]any)
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			out = append(out, it.(map[string]any))
		}
		return out
	}
	byKey := func(list, key string) map[string]any {
		t.Helper()
		items := entries(admin, list, "?filter[key]="+key)
		if len(items) != 1 {
			t.Fatalf("entry %s of %s: %v", key, list, items)
		}
		return items[0]
	}
	red, hq, de := byKey("probe.color", "red"), byKey("probe.place", "duesseldorf_hq"), byKey("probe.place", "de")
	if hq["full_name"] != "Deutschland > Düsseldorf > Hauptgebäude" {
		t.Fatalf("full name: %v", hq["full_name"])
	}

	// A reference to a list refuses an entry of another list, and takes one of its own.
	r = call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "lists", "color": "`+hq["id"].(string)+`"}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, `"field":"color"`) {
		t.Fatalf("foreign entry: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "lists", "color": "`+red["id"].(string)+`", "place": "`+hq["id"].(string)+`"}`, nil)
	if r.code != 201 || r.body["color"] != red["id"] {
		t.Fatalf("record with list values: %d %s", r.code, r.raw)
	}
	// The subtree below Deutschland: Düsseldorf and the Hauptgebäude.
	if items := entries(admin, "probe.place", "?filter[under]="+de["id"].(string)); len(items) != 2 {
		t.Fatalf("under de: %v", items)
	}
	r = call(t, admin, http.MethodPost, "/api/v1/records/probe.place:query", `{"where": {"field": "parent", "under": "`+de["id"].(string)+`"}, "fields": ["full_name"], "sort": ["name"]}`, nil)
	if items, _ := r.body["items"].([]any); r.code != 200 || len(items) != 2 || items[0].(map[string]any)["full_name"] != "Deutschland > Düsseldorf" {
		t.Fatalf("query under: %d %s", r.code, r.raw)
	}
	// Moving the Hauptgebäude below Deutschland shortens its full name; a cycle is refused.
	r = call(t, admin, http.MethodPatch, "/api/v1/records/probe.place/"+hq["id"].(string), `{"parent": "`+de["id"].(string)+`"}`, map[string]string{"If-Match": `"1"`})
	if r.code != 200 || r.body["full_name"] != "Deutschland > Hauptgebäude" {
		t.Fatalf("move: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodPatch, "/api/v1/records/probe.place/"+de["id"].(string), `{"parent": "`+hq["id"].(string)+`"}`, map[string]string{"If-Match": `"1"`})
	if r.code != 422 || !strings.Contains(r.raw, "cycle") {
		t.Fatalf("cycle: %d %s", r.code, r.raw)
	}

	// A person without any role reads every list, sees them in the type listing, and may not
	// write them or read other types.
	acc, err := admin.auth.CreateAccount(ctx, "apitest_lists", "Lists", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "Lists"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_lists", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	nobody := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if items := entries(nobody, "probe.color", ""); len(items) != 3 {
		t.Fatalf("colours for a person without roles: %v", items)
	}
	if r := call(t, nobody, http.MethodGet, "/api/v1/records/probe.place/"+hq["id"].(string), "", nil); r.code != 200 || r.body["full_name"] == nil {
		t.Fatalf("place for a person without roles: %d %s", r.code, r.raw)
	}
	if r := call(t, nobody, http.MethodGet, "/api/v1/types", "", nil); r.code != 200 || !strings.Contains(r.raw, `"probe.color"`) && !strings.Contains(r.raw, `"name":"color"`) || strings.Contains(r.raw, `"name":"probe"`) {
		t.Fatalf("types for a person without roles: %d %s", r.code, r.raw)
	}
	if r := call(t, nobody, http.MethodGet, "/api/v1/records/probe.probe", "", nil); r.code != 200 || len(r.body["items"].([]any)) != 0 {
		t.Fatalf("probe records for a person without roles: %d %s", r.code, r.raw)
	}
	if r := call(t, nobody, http.MethodPost, "/api/v1/records/probe.color", `{"name": "Gelb"}`, nil); r.code != 403 {
		t.Fatalf("writing a list without a role: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.color", `{"name": "Gelb"}`, nil); r.code != 201 {
		t.Fatalf("writing a list as admin: %d %s", r.code, r.raw)
	}
}
