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
)

// The Prüfweg of PL7: persons and groups are records of the core package; a group with
// subgroups answers its members flat or recursively; a person carries further addresses.
func TestPersonsAndGroupsOverHTTP(t *testing.T) {
	api := apiServer(t)
	create := func(typeName, body string) map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body
	}
	suffix := t.Name()

	// Persons with supervisor and a second address.
	boss := create("core.person", `{"last_name": "Chef `+suffix+`", "first_name": "Carla", "email": "carla@example.org", "title": "Dr."}`)
	anna := create("core.person", `{"last_name": "Anna `+suffix+`", "email": "anna@example.org", "supervisor": "`+boss["id"].(string)+`"}`)
	ben := create("core.person", `{"last_name": "Ben `+suffix+`", "phone": "+49 30 1", "mobile": "+49 170 1"}`)
	if anna["supervisor"] != boss["id"] || anna["language"] != "de" || anna["active"] != true {
		t.Fatalf("person defaults: %v", anna)
	}
	create("core.person_email", `{"person": "`+anna["id"].(string)+`", "address": "anna.privat@example.org", "kind": "private"}`)
	r := call(t, api, http.MethodGet, "/api/v1/records/core.person_email?filter[person]="+anna["id"].(string), "", nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(r.raw), &page); err != nil || len(page.Items) != 1 || page.Items[0]["kind"] != "private" {
		t.Fatalf("addresses of anna: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/records/core.person", `{"first_name": "ohne Nachnamen"}`, nil); r.code != 422 {
		t.Fatalf("person without last name: %d %s", r.code, r.raw)
	}

	// A tree of groups: it > support > frontline, with members on every level.
	it := create("core.group", `{"name": "IT `+suffix+`", "purpose": "Alles mit Stecker", "responsible": "`+boss["id"].(string)+`"}`)
	support := create("core.group", `{"name": "Support `+suffix+`", "parent": "`+it["id"].(string)+`"}`)
	frontline := create("core.group", `{"name": "Frontline `+suffix+`", "parent": "`+support["id"].(string)+`"}`)
	create("core.group_member", `{"group": "`+it["id"].(string)+`", "person": "`+boss["id"].(string)+`"}`)
	create("core.group_member", `{"group": "`+support["id"].(string)+`", "person": "`+anna["id"].(string)+`"}`)
	create("core.group_member", `{"group": "`+frontline["id"].(string)+`", "person": "`+ben["id"].(string)+`"}`)
	if r = call(t, api, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+it["id"].(string)+`"}`, nil); r.code != 422 {
		t.Fatalf("member without person: %d %s", r.code, r.raw)
	}

	members := func(groupID string, recursive bool) []map[string]any {
		t.Helper()
		path := "/api/v1/groups/" + groupID + "/members"
		if recursive {
			path += "?recursive=true"
		}
		r := call(t, api, http.MethodGet, path, "", nil)
		if r.code != 200 {
			t.Fatalf("members: %d %s", r.code, r.raw)
		}
		var out []map[string]any
		if err := json.Unmarshal([]byte(r.raw), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	flat := members(it["id"].(string), false)
	if len(flat) != 1 || flat[0]["last_name"] != boss["last_name"] {
		t.Fatalf("flat members of IT: %v", flat)
	}
	deep := members(it["id"].(string), true)
	if len(deep) != 3 || deep[0]["group_name"] != it["name"] || deep[1]["group_name"] != support["name"] || deep[2]["group_name"] != frontline["name"] {
		t.Fatalf("recursive members of IT: %v", deep)
	}
	if deep[2]["person_id"] != ben["id"] || deep[1]["email"] != "anna@example.org" {
		t.Fatalf("recursive members carry person and email: %v", deep)
	}
	if got := members(support["id"].(string), true); len(got) != 2 {
		t.Fatalf("recursive members of Support: %v", got)
	}

	// A trashed membership and a trashed person drop out; an unknown group is 404.
	r = call(t, api, http.MethodGet, "/api/v1/records/core.group_member?filter[person]="+ben["id"].(string), "", nil)
	_ = json.Unmarshal([]byte(r.raw), &page)
	if len(page.Items) != 1 {
		t.Fatalf("membership of ben: %s", r.raw)
	}
	if r = call(t, api, http.MethodDelete, "/api/v1/records/core.group_member/"+page.Items[0]["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("trash membership: %d %s", r.code, r.raw)
	}
	if got := members(it["id"].(string), true); len(got) != 2 {
		t.Fatalf("after trashing ben's membership: %v", got)
	}
	if r = call(t, api, http.MethodDelete, "/api/v1/records/core.person/"+anna["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("trash anna: %d %s", r.code, r.raw)
	}
	if got := members(it["id"].(string), true); len(got) != 1 {
		t.Fatalf("after trashing anna: %v", got)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/groups/01a15a3c-8545-7691-b780-9b915e555b1c/members", "", nil); r.code != 404 {
		t.Fatalf("unknown group: %d %s", r.code, r.raw)
	}
}
