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
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The batch of UI6 over HTTP: one call, a result per row; set, transition, roles and trash
// go through like single calls; a stale version is a conflict, a transition that does not
// apply refused, a value out of range invalid, an unknown record not found; the rows stand
// alone; someone without update gets forbidden per row.
func TestBatchOverHTTP(t *testing.T) {
	admin := apiServer(t)
	suffix := uuid.NewString()[:8]
	ids := make([]string, 4)
	for i := range ids {
		r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", fmt.Sprintf(`{"title": "UI6 %s %d"}`, suffix, i), nil)
		if r.code != 201 {
			t.Fatalf("record: %d %s", r.code, r.raw)
		}
		ids[i] = r.body["id"].(string)
	}
	person := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "UI6 `+suffix+`"}`, nil).body["id"].(string)
	batch := func(api *testAPI, rows string) []map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/probe.probe:batch", `{"rows": [`+rows+`]}`, nil)
		if r.code != 200 {
			t.Fatalf("batch: %d %s", r.code, r.raw)
		}
		var out struct {
			Results []map[string]any `json:"results"`
		}
		if err := json.Unmarshal([]byte(r.raw), &out); err != nil {
			t.Fatal(err)
		}
		return out.Results
	}
	res := batch(admin, strings.Join([]string{
		`{"id": "` + ids[0] + `", "version": 1, "set": {"kind": "fault"}}`,
		`{"id": "` + ids[1] + `", "version": 7, "set": {"priority": 2}}`,
		`{"id": "` + ids[2] + `", "version": 1, "transition": "start"}`,
		`{"id": "` + ids[3] + `", "version": 1, "transition": "finish"}`,
		`{"id": "` + ids[3] + `", "version": 1, "trash": true}`,
		`{"id": "` + ids[0] + `", "version": 2, "set": {"actors": {"requester": [{"type": "core.person", "id": "` + person + `"}]}}}`,
		`{"id": "` + ids[1] + `", "version": 1, "set": {"priority": 99}}`,
		`{"id": "` + uuid.NewString() + `", "version": 1, "set": {"priority": 2}}`,
		`{"id": "` + ids[1] + `", "version": 1, "set": {"priority": 2}, "trash": true}`,
	}, ","))
	want := []string{"ok", "conflict", "ok", "refused", "ok", "ok", "invalid", "not_found", "invalid"}
	for i, w := range want {
		if res[i]["status"] != w {
			t.Fatalf("row %d: %v, want %s (all: %v)", i, res[i], w, res)
		}
	}
	if res[0]["version"] != 2.0 || res[5]["version"] != 3.0 || !strings.Contains(fmt.Sprint(res[6]["problems"]), "priority") {
		t.Fatalf("versions and problems: %v", res)
	}

	// What the rows did, as single calls would have.
	r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+ids[0], "", nil)
	if r.body["kind"] != "fault" || !strings.Contains(r.raw, person) {
		t.Fatalf("first record: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+ids[2], "", nil); r.body["state"] != "in_progress" {
		t.Fatalf("transition: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+ids[3], "", nil); r.code != 404 {
		t.Fatalf("trash: %d", r.code)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+ids[1], "", nil); r.body["version"] != 1.0 {
		t.Fatalf("the refused rows changed the second record: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+ids[0]+"/history", "", nil); !strings.Contains(r.raw, `"field":"kind"`) ||
		!strings.Contains(r.raw, `"field":"actors.requester"`) {
		t.Fatalf("history: %s", r.raw)
	}

	// Someone who may only read gets forbidden per row.
	const role = "ui6_reader"
	call(t, admin, http.MethodDelete, "/api/v1/roles/"+role, "", nil)
	call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "UI6"}}`, nil)
	call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", `{"type_name": "probe.probe", "action": "read", "scope": "all"}`, nil)
	reader := timelineLogin(t, admin, "apitest_ui6", role, `{"last_name": "UI6 Leser"}`)
	if res := batch(reader.api, `{"id": "`+ids[1]+`", "version": 1, "set": {"priority": 2}}`); res[0]["status"] != "forbidden" {
		t.Fatalf("reader: %v", res)
	}

	// Empty and too large batches are refused as a whole.
	if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe:batch", `{"rows": []}`, nil); r.code != 422 {
		t.Fatalf("empty: %d %s", r.code, r.raw)
	}
	many := make([]string, 1001)
	for i := range many {
		many[i] = `{"id": "` + ids[1] + `", "version": 1, "set": {"priority": 2}}`
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe:batch", `{"rows": [`+strings.Join(many, ",")+`]}`, nil); r.code != 422 {
		t.Fatalf("1001 rows: %d", r.code)
	}
}
