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
)

// Portal and workplace (UI10): /auth/me says who may use the workplace (the right workplace
// on core.ui, the administrator through *) and names the person; the type tells the portal
// its fields.
func TestPortalOverHTTP(t *testing.T) {
	admin := apiServer(t)
	timelineRoles(t, admin, "ui10")
	user := timelineLogin(t, admin, "apitest_ui10", "ui10_requester", `{"last_name": "UI10"}`)
	if r := call(t, admin, http.MethodGet, "/api/v1/auth/me", "", nil); r.body["workplace"] != true || r.body["person_id"] != admin.person.String() {
		t.Fatalf("admin: %s", r.raw)
	}
	if r := call(t, user.api, http.MethodGet, "/api/v1/auth/me", "", nil); r.body["workplace"] != false || r.body["person_id"] != user.person {
		t.Fatalf("user without workplace: %s", r.raw)
	}
	// With the right the same person is an agent.
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/ui10_requester/permissions", `{"type_name": "core.ui", "action": "workplace", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("grant: %d %s", r.code, r.raw)
	}
	deadline := 0
	for {
		r := call(t, user.api, http.MethodGet, "/api/v1/auth/me", "", nil)
		if r.body["workplace"] == true {
			break
		}
		if deadline++; deadline > 50 {
			t.Fatalf("the right does not arrive: %s", r.raw)
		}
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil); !strings.Contains(r.raw, `"portal":{"fields":["title","kind"]}`) {
		t.Fatalf("type: %s", r.raw)
	}
}
