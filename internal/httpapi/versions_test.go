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
)

// Versions over HTTP (PL19): list, one version, comparison, restore with If-Match; a state
// change through a transition is part of the versions.
func TestVersionsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "first", "priority": 2}`, nil)
	if rec.code != 201 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, admin, http.MethodPatch, base, `{"title": "second"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("update: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, base+":transition", `{"transition": "start"}`, nil); r.code != 200 {
		t.Fatalf("transition: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, base, `{"priority": 4}`, map[string]string{"If-Match": `"3"`}); r.code != 200 {
		t.Fatalf("update: %d %s", r.code, r.raw)
	}

	r := call(t, admin, http.MethodGet, base+"/versions", "", nil)
	var versions []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &versions)
	// Each version names who wrote it (SD16b).
	if r.code != 200 || len(versions) != 4 || versions[2]["actions"].([]any)[0] != "transition" || versions[0]["actor_name"] != "API Test" {
		t.Fatalf("versions: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/versions/1", "", nil); r.code != 200 || r.body["title"] != "first" || r.body["state"] != "new" {
		t.Fatalf("version 1: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodGet, base+"/compare?from=1&to=4", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `{"field":"title","from":"first","to":"second"}`) ||
		!strings.Contains(r.raw, `{"field":"priority","from":2,"to":4}`) || !strings.Contains(r.raw, `{"field":"state","from":"new","to":"in_progress"}`) {
		t.Fatalf("compare: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/versions/9", "", nil); r.code != 404 {
		t.Fatalf("version 9: %d %s", r.code, r.raw)
	}

	// Restore: without If-Match 428; with it the fields of version 1 come back, the state stays.
	if r := call(t, admin, http.MethodPost, base+"/versions/1:restore", "", nil); r.code != 428 {
		t.Fatalf("restore without If-Match: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodPost, base+"/versions/1:restore", "", map[string]string{"If-Match": `"4"`})
	if r.code != 200 || r.body["title"] != "first" || r.body["priority"] != 2.0 || r.body["state"] != "in_progress" || r.body["version"] != 5.0 || r.hdr.Get("ETag") != `"5"` {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}

	// A type without history has no versions, and its OpenAPI no such paths.
	p := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths, _ := p.body["paths"].(map[string]any)
	if paths["/api/v1/records/probe.probe/{id}/versions/{n}:restore"] == nil || paths["/api/v1/records/probe.probe/{id}/compare"] == nil {
		t.Fatal("version paths of probe.probe missing in the OpenAPI")
	}
}
