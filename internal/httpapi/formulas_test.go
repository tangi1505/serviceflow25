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

// Calculated fields over HTTP (PL22): the ITIL matrix of the probe type follows impact and
// urgency, cannot be written and is read-only in the OpenAPI.
func TestCalculatedFieldsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "matrix", "impact": "high", "urgency": "low"}`, nil)
	if rec.code != 201 || rec.body["itil_priority"] != 3.0 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, admin, http.MethodPatch, base, `{"urgency": "high"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 || r.body["itil_priority"] != 1.0 {
		t.Fatalf("update: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, base, `{"urgency": null}`, map[string]string{"If-Match": `"2"`}); r.code != 200 || r.body["itil_priority"] != nil {
		t.Fatalf("empty urgency: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, base, `{"itil_priority": 1}`, map[string]string{"If-Match": `"3"`}); r.code != 422 || !strings.Contains(r.raw, "read_only") {
		t.Fatalf("writing a calculated field: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil); !strings.Contains(r.raw, `"name":"itil_priority"`) || !strings.Contains(r.raw, `"formula":`) {
		t.Fatalf("type: %s", r.raw)
	}
	p := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	f := p.body["components"].(map[string]any)["schemas"].(map[string]any)["probe.probe"].(map[string]any)["properties"].(map[string]any)["itil_priority"].(map[string]any)
	if f["readOnly"] != true || !strings.HasPrefix(f["description"].(string), "calculated: ") {
		t.Fatalf("itil_priority in the OpenAPI: %v", f)
	}
}
