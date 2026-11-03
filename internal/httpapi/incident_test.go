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
	"os"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
)

// The Prüfweg of SD2 (SD-Plan E5): the incident takes the trait ticket of the package sd
// (N7) and its number INC-…; impact "mittel" and urgency "hoch" give P2; resolving without a
// solution is 422 at the field; the requester alone confirms or refuses (N8), a refusal needs
// its reason and leaves it as an entry; closed is final.
func TestIncidentOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd2"))
	admin := apiServer(t)
	for _, dir := range []string{"../../packages/sd", "../../packages/incident"} {
		installPackage(t, t.Context(), admin.rt, dir)
	}
	const inc = "/api/v1/records/incident.incident"

	// The type has the fields and roles of the trait.
	ty := call(t, admin, http.MethodGet, "/api/v1/types/incident.incident", "", nil)
	if ty.code != 200 || !strings.Contains(ty.raw, `"sd.category"`) || !strings.Contains(ty.raw, `"requester"`) {
		t.Fatalf("type: %d %.400s", ty.code, ty.raw)
	}

	// The requester: a person with the right to read and answer what they are involved in, as
	// in the portal.
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd2_portal", "label": {"de": "SD2 Portal"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, action := range []string{"read", "comment"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/sd2_portal/permissions",
			`{"type_name": "incident.incident", "action": "`+action+`", "scope": "involved"}`, nil); r.code != 204 {
			t.Fatalf("permission %s: %d %s", action, r.code, r.raw)
		}
	}
	req := timelineLogin(t, admin, "apitest_sd2_requester", "sd2_portal", `{"last_name": "Anforderin"}`)

	// Impact medium, urgency high: P2; the number INC-1.
	kinds := call(t, admin, http.MethodGet, "/api/v1/records/sd.solution_kind?limit=50", "", nil)
	fixed := ""
	for _, it := range kinds.body["items"].([]any) {
		if k := it.(map[string]any); k["key"] == "fixed" {
			fixed = k["id"].(string)
		}
	}
	r := call(t, admin, http.MethodPost, inc, `{"title": "Drucker druckt leer", "impact": "medium", "urgency": "high",
		"actors": {"requester": [{"type": "core.person", "id": "`+req.person+`"}]}}`, nil)
	if r.code != 201 || r.body["priority"] != 2.0 || r.body["number"] != 1.0 {
		t.Fatalf("new incident: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	transition := func(api *testAPI, body string) resp {
		t.Helper()
		return call(t, api, http.MethodPost, inc+"/"+id+":transition", body, nil)
	}
	version := func() string {
		return fmt.Sprintf(`"%v"`, call(t, admin, http.MethodGet, inc+"/"+id, "", nil).body["version"])
	}

	// Resolving without a solution: 422 at the field solution.
	if r := call(t, admin, http.MethodPost, inc+"/"+id+":transition", `{"transition": "resolve", "fields": {}}`,
		map[string]string{"If-Match": version()}); r.code != 422 || !strings.Contains(r.raw, `"solution"`) {
		t.Fatalf("resolve without a solution: %d %s", r.code, r.raw)
	}
	resolve := func() {
		t.Helper()
		r := call(t, admin, http.MethodPost, inc+"/"+id+":transition",
			`{"transition": "resolve", "fields": {"solution": "Treiber neu installiert", "solution_kind": "`+fixed+`"}}`,
			map[string]string{"If-Match": version()})
		if r.code != 200 || r.body["state"] != "resolved" {
			t.Fatalf("resolve: %d %s", r.code, r.raw)
		}
	}
	resolve()

	// Only the requester may confirm or refuse; the agent gets 403, the list says why.
	if r := transition(admin, `{"transition": "confirm"}`); r.code != 403 {
		t.Fatalf("the agent confirms: %d %s", r.code, r.raw)
	}
	offered := func(api *testAPI) map[string]map[string]any {
		t.Helper()
		r := call(t, api, http.MethodGet, inc+"/"+id+"/transitions", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("transitions: %d %s", r.code, r.raw)
		}
		out := map[string]map[string]any{}
		for _, it := range list {
			out[it["name"].(string)] = it
		}
		return out
	}
	if o := offered(admin)["confirm"]; o["allowed"] != false || o["reason"] != "role" {
		t.Fatalf("confirm for the agent: %v", o)
	}
	if o := offered(req.api); o["confirm"]["allowed"] != true || o["refuse"]["allowed"] != true || o["refuse"]["comment"] != "required" {
		t.Fatalf("for the requester: %v", o)
	}

	// Refusing needs its reason; with it the incident is in progress again and the reason an
	// entry of the requester.
	if r := transition(req.api, `{"transition": "refuse"}`); r.code != 422 || !strings.Contains(r.raw, `"comment"`) {
		t.Fatalf("refuse without a reason: %d %s", r.code, r.raw)
	}
	if r := transition(req.api, `{"transition": "refuse", "comment": "Er druckt immer noch leer."}`); r.code != 200 || r.body["state"] != "in_progress" {
		t.Fatalf("refuse: %d %s", r.code, r.raw)
	}
	found := false
	for _, it := range timeline(t, admin, inc+"/"+id) {
		if e, ok := it["entry"].(map[string]any); ok && e["body"] == "Er druckt immer noch leer." && e["created_by"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatal("the reason is no entry at the incident")
	}
	if r := transition(req.api, `{"transition": "confirm", "comment": "x"}`); r.code != 422 && r.code != 409 {
		t.Fatalf("confirm with a text or from in progress: %d %s", r.code, r.raw)
	}

	// Resolved again, the requester confirms: closed, and that is final.
	resolve()
	if r := transition(req.api, `{"transition": "confirm"}`); r.code != 200 || r.body["state"] != "closed" {
		t.Fatalf("confirm: %d %s", r.code, r.raw)
	}
	if r := transition(admin, `{"transition": "reopen"}`); r.code != 409 {
		t.Fatalf("reopen a closed incident: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, inc+"/"+id, `{"title": "anders"}`, map[string]string{"If-Match": version()}); r.code != 422 {
		t.Fatalf("change a closed incident: %d %s", r.code, r.raw)
	}
}
