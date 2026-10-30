// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authn/authntest"
)

// The Prüfweg of PL12 against a test directory: persons and groups arrive with their
// memberships, a rename stays the same record, a vanished person is deactivated (not
// deleted) and her account blocked, a returning one reactivated; the API manages the
// directory and queues a sync; an OIDC sign-in links the synced person by email.
func TestDirectorySync(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	for _, sql := range []string{
		"DELETE FROM ldap_directory WHERE name LIKE 'apitest%'",
		"DELETE FROM account WHERE username IN ('lena', 'apitest_lena')",
		"DELETE FROM rec_core_group_member WHERE \"group\" IN (SELECT id FROM rec_core_group WHERE name LIKE 'apitest-%')",
		"DELETE FROM rec_core_group WHERE name LIKE 'apitest-%'",
		"DELETE FROM rec_core_person WHERE email LIKE '%@apitest.example.org'",
	} {
		if _, err := admin.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	dir := authntest.StartLDAP(t)
	lena := authntest.Person{ID: "11111111-1111-1111-1111-111111111111", UID: "lena", FirstName: "Lena", LastName: "Testerin", Email: "lena@apitest.example.org"}
	lutz := authntest.Person{ID: "22222222-2222-2222-2222-222222222222", UID: "lutz", FirstName: "Lutz", LastName: "Nutzer", Email: "lutz@apitest.example.org"}
	dir.SetPersons(lena, lutz)
	dir.SetGroups(authntest.Group{ID: "aaaaaaaa-1111-1111-1111-111111111111", Name: "apitest-admins", Members: []string{"lena"}},
		authntest.Group{ID: "aaaaaaaa-2222-2222-2222-222222222222", Name: "apitest-users", Members: []string{"lena", "lutz"}})
	t.Setenv(authn.PasswordEnv("apitest"), dir.Password)

	// The directory through the API.
	body := fmt.Sprintf(`{"name": "apitest", "url": %q, "bind_dn": %q, "base_dn": %q, "page_size": 0}`, dir.URL, dir.BindDN, dir.BaseDN)
	r := call(t, admin, http.MethodPost, "/api/v1/directories", body, nil)
	if r.code != 201 || r.body["person_filter"] != "(objectClass=person)" || r.body["org_path"] != "root" || r.body["enabled"] != true {
		t.Fatalf("create directory: %d %s", r.code, r.raw)
	}
	for _, bad := range []string{
		`{"name": "apitest2", "url": "http://x", "bind_dn": "a", "base_dn": "b"}`,
		`{"name": "apitest2", "url": "ldap://x", "bind_dn": "a", "base_dn": "b", "person_filter": "(("}`,
		`{"name": "apitest2", "url": "ldap://x", "bind_dn": "a", "base_dn": "b", "org_path": "root.nowhere"}`,
	} {
		if r := call(t, admin, http.MethodPost, "/api/v1/directories", bad, nil); r.code != 422 {
			t.Fatalf("bad directory %s: %d %s", bad, r.code, r.raw)
		}
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/directories/nope", "", nil); r.code != 404 {
		t.Fatalf("unknown directory: %d", r.code)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/directories/apitest:sync", "", nil); r.code != 202 || r.body["kind"] != "ldap.sync" || r.body["queue"] != "bulk" {
		t.Fatalf("queue sync: %d %s", r.code, r.raw)
	}

	// The sync itself, in the foreground as `itsm ldap sync` would run it.
	res, err := admin.authn.Sync(ctx, "apitest")
	if err != nil {
		t.Fatal(err)
	}
	if res.Persons.Seen != 2 || res.Persons.Created != 2 || res.Groups.Created != 2 || res.Members.Added != 3 || len(res.Warnings) != 0 {
		t.Fatalf("first sync: %+v", res)
	}
	person := func(email string) map[string]any {
		t.Helper()
		r := call(t, admin, http.MethodGet, "/api/v1/records/core.person?filter[email]="+email, "", nil)
		var page struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal([]byte(r.raw), &page)
		if r.code != 200 || len(page.Items) != 1 {
			t.Fatalf("person %s: %d %s", email, r.code, r.raw)
		}
		return page.Items[0]
	}
	p := person("lena@apitest.example.org")
	if p["first_name"] != "Lena" || p["last_name"] != "Testerin" || p["active"] != true {
		t.Fatalf("lena: %v", p)
	}
	lenaID := p["id"].(string)
	members := func(group string) []string {
		t.Helper()
		r := call(t, admin, http.MethodGet, "/api/v1/records/core.group?filter[name]="+group, "", nil)
		var page struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal([]byte(r.raw), &page)
		if r.code != 200 || len(page.Items) != 1 {
			t.Fatalf("group %s: %d %s", group, r.code, r.raw)
		}
		r = call(t, admin, http.MethodGet, "/api/v1/groups/"+page.Items[0]["id"].(string)+"/members", "", nil)
		var list []map[string]any
		_ = json.Unmarshal([]byte(r.raw), &list)
		var out []string
		for _, m := range list {
			out = append(out, fmt.Sprint(m["last_name"]))
		}
		return out
	}
	if m := members("apitest-users"); len(m) != 2 {
		t.Fatalf("members of apitest-users: %v", m)
	}
	if m := members("apitest-admins"); len(m) != 1 || m[0] != "Testerin" {
		t.Fatalf("members of apitest-admins: %v", m)
	}

	// A second run changes nothing; a rename updates the same record.
	res, err = admin.authn.Sync(ctx, "apitest")
	if err != nil || res.Persons.Created != 0 || res.Persons.Updated != 0 || res.Members.Added != 0 || res.Members.Removed != 0 {
		t.Fatalf("second sync: %+v %v", res, err)
	}
	lena.LastName = "Musterfrau"
	dir.SetPersons(lena, lutz)
	res, err = admin.authn.Sync(ctx, "apitest")
	if err != nil || res.Persons.Updated != 1 || res.Persons.Created != 0 {
		t.Fatalf("rename: %+v %v", res, err)
	}
	if p := person("lena@apitest.example.org"); p["id"] != lenaID || p["last_name"] != "Musterfrau" {
		t.Fatalf("renamed person must be the same record: %v", p)
	}

	// An account of lena; then lena vanishes from the directory: deactivated, not deleted,
	// the account blocked, the membership gone.
	acc, err := admin.auth.CreateAccount(ctx, "lena", "Lena", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	pid := uuid.MustParse(lenaID)
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	dir.SetPersons(lutz)
	dir.SetGroups(authntest.Group{ID: "aaaaaaaa-1111-1111-1111-111111111111", Name: "apitest-admins"},
		authntest.Group{ID: "aaaaaaaa-2222-2222-2222-222222222222", Name: "apitest-users", Members: []string{"lutz"}})
	res, err = admin.authn.Sync(ctx, "apitest")
	if err != nil || res.Persons.Deactivated != 1 || res.Members.Removed != 2 {
		t.Fatalf("vanish: %+v %v", res, err)
	}
	if p := person("lena@apitest.example.org"); p["id"] != lenaID || p["active"] != false {
		t.Fatalf("vanished person must stay, inactive: %v", p)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "lena", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""}); r.code != 401 {
		t.Fatalf("blocked account: %d %s", r.code, r.raw)
	}
	if m := members("apitest-users"); len(m) != 1 || m[0] != "Nutzer" {
		t.Fatalf("members after vanish: %v", m)
	}

	// Lena returns: reactivated, same record.
	dir.SetPersons(lena, lutz)
	res, err = admin.authn.Sync(ctx, "apitest")
	if err != nil || res.Persons.Reactivated != 1 || res.Persons.Created != 0 {
		t.Fatalf("return: %+v %v", res, err)
	}
	if p := person("lena@apitest.example.org"); p["id"] != lenaID || p["active"] != true {
		t.Fatalf("returned person: %v", p)
	}
	d := call(t, admin, http.MethodGet, "/api/v1/directories/apitest", "", nil)
	if d.code != 200 || d.body["last_sync_at"] == nil || d.body["last_result"] == nil {
		t.Fatalf("directory after sync: %d %s", d.code, d.raw)
	}

	// Wrong password: the sync fails without touching anything.
	t.Setenv(authn.PasswordEnv("apitest"), "wrong")
	if _, err := admin.authn.Sync(ctx, "apitest"); err == nil {
		t.Fatal("a wrong bind password must fail the sync")
	}
	t.Setenv(authn.PasswordEnv("apitest"), dir.Password)

	// An OIDC sign-in with lutz's email links the synced person instead of creating one.
	admin.idp.Login(authntest.User{Subject: "ldap-lutz", Username: "apitest_lutz", Name: "Lutz Nutzer", Email: "lutz@apitest.example.org"})
	browser := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	hop := func(target string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := browser.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 302 {
			t.Fatalf("%s: %d", target, res.StatusCode)
		}
		return res
	}
	start := hop(admin.URL + "/api/v1/auth/oidc/start")
	provider := hop(start.Header.Get("Location"))
	done := hop(provider.Header.Get("Location"))
	if done.Header.Get("Set-Cookie") == "" {
		t.Fatalf("callback: %s", done.Header.Get("Location"))
	}
	lutzPerson := person("lutz@apitest.example.org")
	var linked uuid.NullUUID
	if err := admin.pool.QueryRow(ctx, "SELECT person_id FROM account WHERE username = 'apitest_lutz'").Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if !linked.Valid || linked.UUID.String() != lutzPerson["id"] {
		t.Fatalf("the account must link the synced person %v, got %v", lutzPerson["id"], linked)
	}

	// Update and delete of the directory.
	if r := call(t, admin, http.MethodPut, "/api/v1/directories/apitest", fmt.Sprintf(`{"url": %q, "bind_dn": %q, "base_dn": %q, "enabled": false}`, dir.URL, dir.BindDN, dir.BaseDN), nil); r.code != 200 || r.body["enabled"] != false || r.body["page_size"] != 0.0 {
		t.Fatalf("update directory: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/directories/apitest", "", nil); r.code != 204 {
		t.Fatalf("delete directory: %d %s", r.code, r.raw)
	}
	if p := person("lena@apitest.example.org"); p["id"] != lenaID {
		t.Fatalf("persons stay after the directory is gone: %v", p)
	}
}
