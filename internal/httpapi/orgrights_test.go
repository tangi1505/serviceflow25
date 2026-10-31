// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// Rights on the organisation tree (found before UI9: every signed-in account could change
// nodes and settings, the password rules among them). Without manage_org and manage_settings
// on core.org nothing changes; with them, assigned at a unit, its subtree only; reading stays
// open.
func TestOrgRightsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	for _, p := range []string{"root.orgrights.sub", "root.orgrights"} {
		call(t, admin, http.MethodDelete, "/api/v1/org/nodes/"+p, "", nil)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "orgrights"}`, nil); r.code != 201 {
		t.Fatalf("node: %d %s", r.code, r.raw)
	}
	defer func() {
		call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.orgrights.sub", "", nil)
		call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.orgrights", "", nil)
	}()
	timelineRoles(t, admin, "orgr")
	reader := timelineLogin(t, admin, "apitest_orgr_reader", "orgr_reader", `{"last_name": "Org Leser"}`)

	// A unit administrator: both rights, assigned at root.orgrights, reaching its subtree.
	const unitRole = "orgr_unit"
	dropRole(t, admin, unitRole)
	call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+unitRole+`", "label": {"de": "Einheit"}}`, nil)
	for _, action := range []string{"manage_org", "manage_settings"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+unitRole+"/permissions", `{"type_name": "core.org", "action": "`+action+`", "scope": "subtree"}`, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	acc, err := admin.auth.CreateAccount(ctx, "apitest_orgr_unit", "Einheit", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "Org Einheit"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "`+unitRole+`", "person_id": "`+pid.String()+`", "org_path": "root.orgrights"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	in := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_orgr_unit", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	unit := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, in.hdr)}

	for _, c := range []struct {
		name         string
		method, path string
		body         string
		reader, unit int
	}{
		{"set at the root", http.MethodPut, "/api/v1/org/nodes/root/settings/auth.password_min_length", `4`, 403, 403},
		{"node under the root", http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "orgrights2"}`, 403, 403},
		{"node in the unit", http.MethodPost, "/api/v1/org/nodes", `{"parent": "root.orgrights", "name": "sub"}`, 403, 201},
		{"set in the unit", http.MethodPut, "/api/v1/org/nodes/root.orgrights.sub/settings/calendar.default", `"x"`, 403, 204},
		{"unset in the unit", http.MethodDelete, "/api/v1/org/nodes/root.orgrights.sub/settings/calendar.default", ``, 403, 204},
		{"delete the root", http.MethodDelete, "/api/v1/org/nodes/root", ``, 403, 403},
	} {
		if r := call(t, reader.api, c.method, c.path, c.body, nil); r.code != c.reader {
			t.Fatalf("reader, %s: %d %s", c.name, r.code, r.raw)
		}
		if r := call(t, unit, c.method, c.path, c.body, nil); r.code != c.unit {
			t.Fatalf("unit, %s: %d %s", c.name, r.code, r.raw)
		}
	}
	// Reading stays open.
	if r := call(t, reader.api, http.MethodGet, "/api/v1/org/nodes/root/settings", "", nil); r.code != 200 {
		t.Fatalf("reader reads settings: %d", r.code)
	}
	if r := call(t, reader.api, http.MethodGet, "/api/v1/org/nodes", "", nil); r.code != 200 {
		t.Fatalf("reader reads nodes: %d", r.code)
	}
}
