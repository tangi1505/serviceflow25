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
	"slices"
	"testing"

	"github.com/google/uuid"
)

// The Prüfweg of PL75: with delete but without purge a person moves a record to the trash and
// takes it out again, but purging is 403; with purge it is 204. The type says which actions
// the session holds (may), so that the interface offers only what goes.
func TestPurgeNeedsItsOwnRight(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const role = "purge_test"
	dropRole(t, admin, role)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "Papierkorb"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	grant := func(action string) {
		t.Helper()
		body := `{"type_name": "probe.probe", "action": "` + action + `", "scope": "all"}`
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", body, nil); r.code != 204 {
			t.Fatalf("permission %s: %d %s", action, r.code, r.raw)
		}
	}
	for _, action := range []string{"read", "create", "delete"} {
		grant(action)
	}

	const name = "apitest_purge"
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
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "`+role+`", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	signIn := func() *testAPI {
		t.Helper()
		login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
		if login.code != 200 {
			t.Fatalf("login: %d %s", login.code, login.raw)
		}
		return &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	}
	may := func(api *testAPI) []string {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
		var def struct {
			May []string `json:"may"`
		}
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &def) != nil {
			t.Fatalf("type: %d %s", r.code, r.raw)
		}
		return def.May
	}

	person := signIn()
	if got := may(person); !slices.Equal(got, []string{"read", "create", "delete"}) {
		t.Fatalf("may without purge: %v", got)
	}
	if got := may(admin); !slices.Equal(got, []string{"read", "create", "update", "delete", "purge"}) {
		t.Fatalf("may of the administration: %v", got)
	}

	rec := call(t, person, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "PL75 Papierkorb"}`, nil)
	if rec.code != 201 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	path := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, person, http.MethodDelete, path, "", nil); r.code != 204 {
		t.Fatalf("trash: %d %s", r.code, r.raw)
	}
	if r := call(t, person, http.MethodPost, path+":purge", "", nil); r.code != 403 {
		t.Fatalf("purge without the right: %d %s", r.code, r.raw)
	}
	if r := call(t, person, http.MethodPost, path+":restore", "", nil); r.code != 204 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	if r := call(t, person, http.MethodDelete, path, "", nil); r.code != 204 {
		t.Fatalf("trash again: %d %s", r.code, r.raw)
	}

	// With purge the same person deletes for good; the session sees the new right after the
	// change of the role.
	grant("purge")
	person = signIn()
	if got := may(person); !slices.Equal(got, []string{"read", "create", "delete", "purge"}) {
		t.Fatalf("may with purge: %v", got)
	}
	if r := call(t, person, http.MethodPost, path+":purge", "", nil); r.code != 204 {
		t.Fatalf("purge with the right: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, path, "", nil); r.code != 404 {
		t.Fatalf("after purge: %d %s", r.code, r.raw)
	}
}
