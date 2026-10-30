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
	"testing/fstest"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// Traits over HTTP (PL17): the trait listing, fields marked with their trait, sub-resources
// only for declared traits (501 until built, 404 otherwise), history only with the trait.
func TestTraitsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()

	r := call(t, admin, http.MethodGet, "/api/v1/traits", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"name":"history"`) || !strings.Contains(r.raw, `"package":"probe","name":"tagged"`) {
		t.Fatalf("traits: %d %s", r.code, r.raw)
	}
	// timeline came with PL42.
	r = call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"traits":["history","documents","costs","tagged","actors","timeline"]`) || !strings.Contains(r.raw, `"name":"tag_no","type":"text","label":{"de":"Inventarnummer","en":"Inventory number"},"max_length":50,"trait":"tagged"`) {
		t.Fatalf("probe type: %d %s", r.code, r.raw)
	}
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "traits", "tag_no": "INV-7"}`, nil)
	if rec.code != 201 || rec.body["tag_no"] != "INV-7" {
		t.Fatalf("record with a trait field: %d %s", rec.code, rec.raw)
	}
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, admin, http.MethodGet, base+"/history", "", nil); r.code != 200 || !strings.Contains(r.raw, `"field":"tag_no"`) {
		t.Fatalf("history: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/documents", "", nil); r.code != 200 || strings.TrimSpace(r.raw) != "[]" {
		t.Fatalf("documents: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/notes", "", nil); r.code != 404 || !strings.Contains(r.raw, "no trait notes") {
		t.Fatalf("notes: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths, _ := r.body["paths"].(map[string]any)
	if paths["/api/v1/records/probe.probe/{id}/documents"] == nil || paths["/api/v1/records/probe.probe/{id}/history"] == nil || paths["/api/v1/records/probe.probe/{id}/notes"] != nil {
		t.Fatalf("openapi paths of probe.probe: documents %v, history %v, notes %v", paths["/api/v1/records/probe.probe/{id}/documents"] != nil, paths["/api/v1/records/probe.probe/{id}/history"] != nil, paths["/api/v1/records/probe.probe/{id}/notes"] != nil)
	}

	// A type without traits: no history, no sub-resources, no such paths in the OpenAPI.
	for _, sql := range []string{"DROP TABLE IF EXISTS rec_apitest_plain", "DELETE FROM package WHERE name = 'apitest'"} {
		if _, err := admin.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte("name: apitest\nversion: 0.1.0\nlabel: {de: A, en: A}\n")},
		"types/plain.yaml": {Data: []byte("name: plain\nlabel: {de: P, en: P}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, admin.pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, admin.pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	plain := call(t, admin, http.MethodPost, "/api/v1/records/apitest.plain", `{"title": "plain"}`, nil)
	if plain.code != 201 {
		t.Fatalf("plain: %d %s", plain.code, plain.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/apitest.plain/"+plain.body["id"].(string)+"/history", "", nil); r.code != 404 {
		t.Fatalf("history without the trait: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths, _ = r.body["paths"].(map[string]any)
	if paths["/api/v1/records/apitest.plain/{id}"] == nil || paths["/api/v1/records/apitest.plain/{id}/history"] != nil {
		t.Fatalf("openapi paths of apitest.plain")
	}
}
