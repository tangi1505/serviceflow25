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
)

// The acceptance of PL14 over HTTP: the query of architektur.md §6.3 (without the relation,
// which PL15 brings) gives exactly the expected records; the same query as a person with a
// smaller scope gives fewer; a bad query is 422 with a problem list.
func TestQueryOverHTTP(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"
	var ids []string
	for i, rec := range []string{
		`{"title": "q new 3", "priority": 3, "kind": "fault"}`,
		`{"title": "q new 4", "priority": 4, "kind": "fault"}`,
		`{"title": "q new 1", "priority": 1, "kind": "fault"}`,
		`{"title": "q question 5", "priority": 5, "kind": "question"}`,
	} {
		r := call(t, admin, http.MethodPost, base, rec, nil)
		if r.code != 201 {
			t.Fatalf("record %d: %d %s", i, r.code, r.raw)
		}
		ids = append(ids, r.body["id"].(string))
	}
	// The third one moves on: state in_progress.
	if r := call(t, admin, http.MethodPost, base+"/"+ids[2]+":transition", `{"transition": "start"}`, nil); r.code != 200 {
		t.Fatalf("transition: %d %s", r.code, r.raw)
	}
	query := `{
	  "where": {"and": [
	    {"field": "state", "in": ["new", "in_progress"]},
	    {"field": "priority", "gte": 3},
	    {"field": "kind", "eq": "fault"},
	    {"field": "title", "starts": "q "}
	  ]},
	  "fields": ["title", "priority", "state"],
	  "sort": ["-priority"],
	  "limit": 50,
	  "count": true
	}`
	r := call(t, admin, http.MethodPost, base+":query", query, nil)
	var page struct {
		Items []map[string]any `json:"items"`
		Total *int64           `json:"total"`
	}
	_ = json.Unmarshal([]byte(r.raw), &page)
	if r.code != 200 || len(page.Items) != 2 || page.Items[0]["title"] != "q new 4" || page.Items[1]["title"] != "q new 3" || page.Total == nil || *page.Total != 2 {
		t.Fatalf("query: %d %s", r.code, r.raw)
	}
	if _, has := page.Items[0]["kind"]; has {
		t.Fatalf("fields must limit the columns: %v", page.Items[0])
	}

	// A person who reads only own records: the same query gives nothing of the admin's.
	call(t, admin, http.MethodDelete, "/api/v1/roles/q_own", "", nil)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "q_own"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, action := range []string{"read", "create"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/q_own/permissions", `{"type_name": "probe.probe", "action": "`+action+`", "scope": "own"}`, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	acc, err := admin.auth.CreateAccount(ctx, "apitest_q", "apitest_q", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "apitest_q"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "q_own", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_q", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	if r := call(t, user, http.MethodPost, base, `{"title": "q mine 9", "priority": 5, "kind": "fault"}`, nil); r.code != 201 {
		t.Fatalf("own record: %d %s", r.code, r.raw)
	}
	r = call(t, user, http.MethodPost, base+":query", query, nil)
	_ = json.Unmarshal([]byte(r.raw), &page)
	if r.code != 200 || len(page.Items) != 1 || page.Items[0]["title"] != "q mine 9" || *page.Total != 1 {
		t.Fatalf("query with own scope: %d %s", r.code, r.raw)
	}
	// Aggregates respect the scope too.
	r = call(t, user, http.MethodPost, base+":query", `{"group": ["kind"], "aggregate": {"n": {"count": "*"}}}`, nil)
	if r.code != 200 || !json.Valid([]byte(r.raw)) || len(r.raw) == 0 || fmt.Sprint(r.body["rows"]) != "[map[kind:fault n:1]]" {
		t.Fatalf("aggregate with own scope: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, base+":query", `{"where": {"field": "title", "like": "x"}}`, nil); r.code != 422 || r.body["errors"] == nil {
		t.Fatalf("bad operator: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, base+":query", `{"where": {"rel": "affects", "to": "ci", "where": {}}}`, nil); r.code != 422 {
		t.Fatalf("rel before PL15: %d %s", r.code, r.raw)
	}
}
