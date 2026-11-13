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

// The organisation tree over HTTP (PL6): nodes below the root, settings that inherit down the
// tree with their source, records that belong to a node and a list filtered by subtree.
func TestOrgTreeOverHTTP(t *testing.T) {
	api := apiServer(t)
	// Leftovers of earlier runs.
	call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.apitest.team", "", nil)
	call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.apitest", "", nil)
	call(t, api, http.MethodDelete, "/api/v1/org/nodes/root/settings/probe.priority", "", nil)

	r := call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "apitest", "label": {"de": "Testabteilung"}}`, nil)
	if r.code != 201 || r.body["path"] != "root.apitest" || r.body["parent"] != "root" {
		t.Fatalf("create node: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "apitest"}`, nil); r.code != 409 {
		t.Fatalf("duplicate node: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "Bad Name"}`, nil); r.code != 422 {
		t.Fatalf("bad name: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root.nope", "name": "x"}`, nil); r.code != 404 {
		t.Fatalf("missing parent: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root.apitest", "name": "team"}`, nil); r.code != 201 {
		t.Fatalf("create grandchild: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, "/api/v1/org/nodes", "", nil)
	var nodes []map[string]any
	if err := json.Unmarshal([]byte(r.raw), &nodes); err != nil || r.code != 200 || len(nodes) < 3 || nodes[0]["path"] != "root" {
		t.Fatalf("list nodes: %d %s", r.code, r.raw)
	}

	// Settings: root sets 3, the child overrides with 1, the grandchild inherits from the child.
	if r = call(t, api, http.MethodPut, "/api/v1/org/nodes/root/settings/probe.priority", `3`, nil); r.code != 204 {
		t.Fatalf("set at root: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPut, "/api/v1/org/nodes/root.apitest/settings/probe.priority", `1`, nil); r.code != 204 {
		t.Fatalf("set at child: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPut, "/api/v1/org/nodes/root.apitest/settings/probe.priority", `{`, nil); r.code != 400 {
		t.Fatalf("bad json: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPut, "/api/v1/org/nodes/root.nope/settings/probe.priority", `1`, nil); r.code != 404 {
		t.Fatalf("set at missing node: %d %s", r.code, r.raw)
	}
	effective := func(path string) map[string][2]string {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/org/nodes/"+path+"/settings", "", nil)
		if r.code != 200 {
			t.Fatalf("settings of %s: %d %s", path, r.code, r.raw)
		}
		var list []struct {
			Key    string          `json:"key"`
			Value  json.RawMessage `json:"value"`
			Source string          `json:"source"`
		}
		if err := json.Unmarshal([]byte(r.raw), &list); err != nil {
			t.Fatal(err)
		}
		out := map[string][2]string{}
		for _, s := range list {
			out[s.Key] = [2]string{string(s.Value), s.Source}
		}
		return out
	}
	if got := effective("root.apitest.team")["probe.priority"]; got != [2]string{"1", "root.apitest"} {
		t.Fatalf("grandchild inherits from child: %v", got)
	}
	if r = call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.apitest/settings/probe.priority", "", nil); r.code != 204 {
		t.Fatalf("unset: %d %s", r.code, r.raw)
	}
	if got := effective("root.apitest.team")["probe.priority"]; got != [2]string{"3", "root"} {
		t.Fatalf("grandchild inherits from root after unset: %v", got)
	}

	// Records belong to a node; the list filters by subtree.
	const base = "/api/v1/records/probe.probe"
	if r = call(t, api, http.MethodPost, base, `{"title": "org", "org_path": "root.nope"}`, nil); r.code != 422 {
		t.Fatalf("unknown node on create: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPost, base, `{"title": "org team", "org_path": "root.apitest.team"}`, nil)
	if r.code != 201 || r.body["org_path"] != "root.apitest.team" {
		t.Fatalf("create in node: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	if r = call(t, api, http.MethodPost, base, `{"title": "org root"}`, nil); r.code != 201 || r.body["org_path"] != "root" {
		t.Fatalf("create at root: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, base+"?filter[org_path]=root.apitest", "", nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(r.raw), &page); err != nil || r.code != 200 || len(page.Items) != 1 || page.Items[0]["id"] != id {
		t.Fatalf("subtree filter: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, base+"?filter[org_path]=nope", "", nil); r.code != 422 {
		t.Fatalf("bad subtree: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"org_path": "root.apitest"}`, map[string]string{"If-Match": `"1"`})
	if r.code != 200 || r.body["org_path"] != "root.apitest" || r.body["version"].(float64) != 2 {
		t.Fatalf("move: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil)
	var hist []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &hist)
	moved := false
	for _, h := range hist {
		if h["field"] == "org_path" && h["action"] == "update" {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("move is not in the history: %s", r.raw)
	}

	// Deleting: not with children, not the root; a leaf goes.
	if r = call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.apitest", "", nil); r.code != 409 {
		t.Fatalf("delete with children: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodDelete, "/api/v1/org/nodes/root", "", nil); r.code != 404 {
		t.Fatalf("delete root: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.apitest.team", "", nil); r.code != 204 {
		t.Fatalf("delete leaf: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/org/nodes/root.apitest.team", "", nil); r.code != 404 {
		t.Fatalf("after delete: %d %s", r.code, r.raw)
	}
}
