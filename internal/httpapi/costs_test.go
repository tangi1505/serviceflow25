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

// Costs over HTTP (PL20): book, list, change with If-Match, remove; the sums on the record
// follow and its ETag stays; the sums cannot be written; the OpenAPI shows them read-only.
func TestCostsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "with costs"}`, nil)
	if rec.code != 201 || rec.body["time_spent"] != 0.0 || rec.body["cost_total"] != 0.0 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	person := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "Costs"}`, nil)

	timeEntry := call(t, admin, http.MethodPost, base+"/costs",
		`{"kind": "time", "minutes": 90, "rate": 70, "spent_on": "2026-10-26", "person": "`+person.body["id"].(string)+`", "description": "Fehlersuche"}`, nil)
	if timeEntry.code != 201 || timeEntry.body["amount"] != 105.0 || timeEntry.hdr.Get("ETag") != `"1"` {
		t.Fatalf("book time: %d %s", timeEntry.code, timeEntry.raw)
	}
	fixed := call(t, admin, http.MethodPost, base+"/costs", `{"kind": "fixed", "amount": 49.9}`, nil)
	material := call(t, admin, http.MethodPost, base+"/costs", `{"kind": "material", "amount": 12.5}`, nil)
	if fixed.code != 201 || material.code != 201 {
		t.Fatalf("book fixed and material: %s %s", fixed.raw, material.raw)
	}
	if r := call(t, admin, http.MethodPost, base+"/costs", `{"kind": "time", "amount": 5}`, nil); r.code != 422 ||
		!strings.Contains(r.raw, `"field":"minutes"`) || !strings.Contains(r.raw, `"field":"amount"`) {
		t.Fatalf("time without minutes, with amount: %d %s", r.code, r.raw)
	}

	r := call(t, admin, http.MethodGet, base, "", nil)
	if r.body["time_spent"] != 90.0 || r.body["cost_total"] != 167.4 || r.hdr.Get("ETag") != `"1"` {
		t.Fatalf("record after three entries: %s %v", r.raw, r.hdr.Get("ETag"))
	}
	r = call(t, admin, http.MethodGet, base+"/costs", "", nil)
	if r.code != 200 || r.body["currency"] != "EUR" || r.body["time_spent"] != 90.0 || r.body["cost_total"] != 167.4 ||
		len(r.body["entries"].([]any)) != 3 {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}

	// Changing an entry needs its version in If-Match.
	entry := base + "/costs/" + fixed.body["id"].(string)
	if r := call(t, admin, http.MethodPatch, entry, `{"amount": 50}`, nil); r.code != 428 {
		t.Fatalf("change without If-Match: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, entry, `{"amount": 50}`, map[string]string{"If-Match": `"7"`}); r.code != 409 {
		t.Fatalf("change with a stale version: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, entry, `{"amount": 50}`, map[string]string{"If-Match": `"1"`}); r.code != 200 || r.hdr.Get("ETag") != `"2"` {
		t.Fatalf("change: %d %s", r.code, r.raw)
	}
	// Removing the time entry: both sums follow.
	if r := call(t, admin, http.MethodDelete, base+"/costs/"+timeEntry.body["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("remove: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base, "", nil); r.body["time_spent"] != 0.0 || r.body["cost_total"] != 62.5 {
		t.Fatalf("record after removing the time entry: %s", r.raw)
	}

	// The record's version never moved; the sums cannot be written.
	if r := call(t, admin, http.MethodPatch, base, `{"title": "still version 1"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("edit after bookings: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, base, `{"cost_total": 0}`, map[string]string{"If-Match": `"2"`}); r.code != 422 || !strings.Contains(r.raw, "read_only") {
		t.Fatalf("writing a sum: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/history", "", nil); !strings.Contains(r.raw, `"action":"cost_add"`) ||
		!strings.Contains(r.raw, `"action":"cost_change"`) || !strings.Contains(r.raw, `"action":"cost_remove"`) {
		t.Fatalf("history: %s", r.raw)
	}
	// Filtering by a sum works like any number field.
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe?filter[cost_total]=62.5", "", nil); r.code != 200 || !strings.Contains(r.raw, rec.body["id"].(string)) {
		t.Fatalf("filter by cost_total: %d %s", r.code, r.raw)
	}

	// A type without the trait has no costs; the OpenAPI shows the paths and read-only sums.
	if r := call(t, admin, http.MethodGet, "/api/v1/records/core.person/"+person.body["id"].(string)+"/costs", "", nil); r.code != 404 {
		t.Fatalf("costs of core.person: %d %s", r.code, r.raw)
	}
	p := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths, _ := p.body["paths"].(map[string]any)
	if paths["/api/v1/records/probe.probe/{id}/costs/{costId}"] == nil || paths["/api/v1/records/core.person/{id}/costs"] != nil {
		t.Fatal("cost paths in the OpenAPI are wrong")
	}
	schemas := p.body["components"].(map[string]any)["schemas"].(map[string]any)
	sum := schemas["probe.probe"].(map[string]any)["properties"].(map[string]any)["cost_total"].(map[string]any)
	if sum["readOnly"] != true {
		t.Fatalf("cost_total in the OpenAPI: %v", sum)
	}
}
