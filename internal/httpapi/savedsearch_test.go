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
	"time"

	"github.com/google/uuid"
)

// The Prüfweg of UI4 over HTTP: Anna saves a search and shares it with Ben; Ben sees it, Carl
// does not; running it, everyone sees only the records they may read. A search that could not
// run is refused when saving.
func TestSavedSearchesOverHTTP(t *testing.T) {
	admin := apiServer(t)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	const role = "ui4_searcher"
	call(t, admin, http.MethodDelete, "/api/v1/roles/"+role, "", nil)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "UI4"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{
		// Probe records: each person reads and writes only their own.
		`{"type_name": "probe.probe", "action": "read", "scope": "own"}`,
		`{"type_name": "probe.probe", "action": "create", "scope": "all"}`,
		// Saved searches: one's own, and those shared with one (the role "shared", WF3).
		`{"type_name": "core.saved_search", "action": "read", "scope": "own"}`,
		`{"type_name": "core.saved_search", "action": "read", "scope": "involved"}`,
		`{"type_name": "core.saved_search", "action": "create", "scope": "all"}`,
		`{"type_name": "core.saved_search", "action": "update", "scope": "own"}`,
	} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission %s: %d %s", p, r.code, r.raw)
		}
	}
	anna := timelineLogin(t, admin, "apitest_ui4_anna", role, `{"last_name": "UI4 Anna"}`)
	ben := timelineLogin(t, admin, "apitest_ui4_ben", role, `{"last_name": "UI4 Ben"}`)
	carl := timelineLogin(t, admin, "apitest_ui4_carl", role, `{"last_name": "UI4 Carl"}`)

	// Each has a fault and a question of their own.
	for _, u := range []timelineUser{anna, ben} {
		for _, kind := range []string{"fault", "question"} {
			if r := call(t, u.api, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "UI4 `+suffix+` `+kind+`", "kind": "`+kind+`"}`, nil); r.code != 201 {
				t.Fatalf("record: %d %s", r.code, r.raw)
			}
		}
	}

	// Wrong searches are refused at the right field.
	for _, c := range []struct{ body, field string }{
		{`"target_type": "probe.nothing"`, "target_type"},
		{`"target_type": "probe.probe", "query": "kind = fault"`, "query"},
		{`"target_type": "probe.probe", "query": "{\"field\": \"nofield\", \"eq\": 1}"`, "query"},
		{`"target_type": "probe.probe", "query": "{\"field\": \"kind\", \"near\": 1}"`, "query"},
		{`"target_type": "probe.probe", "columns": "title, nofield"`, "columns"},
		{`"target_type": "probe.probe", "sort": "-nofield"`, "sort"},
	} {
		r := call(t, anna.api, http.MethodPost, "/api/v1/records/core.saved_search", `{"name": "falsch", `+c.body+`}`, nil)
		if r.code != 422 || !strings.Contains(r.raw, `"field":"`+c.field+`"`) {
			t.Fatalf("wrong search %s: %d %s", c.body, r.code, r.raw)
		}
	}

	// Anna saves the faults of this run and shares the search with Ben.
	where, _ := json.Marshal(map[string]any{"and": []any{
		map[string]any{"field": "kind", "eq": "fault"},
		map[string]any{"field": "title", "starts": "UI4 " + suffix},
	}})
	whereText, _ := json.Marshal(string(where))
	s := call(t, anna.api, http.MethodPost, "/api/v1/records/core.saved_search", `{"name": "Störungen `+suffix+`",
		"target_type": "probe.probe", "query": `+string(whereText)+`, "columns": "title, kind, updated_at", "sort": "-updated_at",
		"actors": {"shared": [{"type": "core.person", "id": "`+ben.person+`"}]}}`, nil)
	if s.code != 201 {
		t.Fatalf("save: %d %s", s.code, s.raw)
	}
	sid := s.body["id"].(string)

	searches := func(u timelineUser) string {
		t.Helper()
		r := call(t, u.api, http.MethodGet, "/api/v1/records/core.saved_search?filter[target_type]=probe.probe&limit=100", "", nil)
		if r.code != 200 {
			t.Fatalf("searches: %d %s", r.code, r.raw)
		}
		return r.raw
	}
	if a, b, c := strings.Contains(searches(anna), sid), strings.Contains(searches(ben), sid), strings.Contains(searches(carl), sid); !a || !b || c {
		t.Fatalf("the search is not visible to Anna and Ben only: Anna %v, Ben %v, Carl %v", a, b, c)
	}
	if r := call(t, carl.api, http.MethodGet, "/api/v1/records/core.saved_search/"+sid, "", nil); r.code != 404 {
		t.Fatalf("Carl reads the search: %d", r.code)
	}
	// Ben may read it, not change it.
	if r := call(t, ben.api, http.MethodPatch, "/api/v1/records/core.saved_search/"+sid, `{"name": "meins"}`, map[string]string{"If-Match": `"1"`}); r.code != 403 && r.code != 404 {
		t.Fatalf("Ben changes the search: %d %s", r.code, r.raw)
	}

	// Run as each one: the condition of the search with the rights of the reader.
	run := func(u timelineUser) []string {
		t.Helper()
		r := call(t, u.api, http.MethodPost, "/api/v1/records/probe.probe:query", `{"where": `+string(where)+`, "sort": ["-updated_at"], "limit": 50}`, nil)
		if r.code != 200 {
			t.Fatalf("run: %d %s", r.code, r.raw)
		}
		var page struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal([]byte(r.raw), &page); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range page.Items {
			out = append(out, it["created_by"].(string))
		}
		return out
	}
	who := func(u timelineUser) string {
		r := call(t, u.api, http.MethodGet, "/api/v1/auth/me", "", nil)
		return r.body["id"].(string)
	}
	if got := run(anna); len(got) != 1 || got[0] != who(anna) {
		t.Fatalf("Anna sees %v", got)
	}
	if got := run(ben); len(got) != 1 || got[0] != who(ben) {
		t.Fatalf("Ben sees %v", got)
	}
	if got := run(timelineUser{api: admin}); len(got) != 2 {
		t.Fatalf("the administrator sees %v", got)
	}

	// One role holds both scopes of read (UI4); removing the one of involved takes the shared
	// search from Ben, while Anna keeps her own.
	r := call(t, admin, http.MethodGet, "/api/v1/roles/"+role, "", nil)
	if strings.Count(r.raw, `"type_name":"core.saved_search","action":"read"`) != 2 {
		t.Fatalf("the role does not hold both scopes: %s", r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/"+role+"/permissions/core.saved_search/read?scope=involved", "", nil); r.code != 204 {
		t.Fatalf("unset involved: %d %s", r.code, r.raw)
	}
	waitFor := func(cond func() bool) {
		t.Helper()
		for i := 0; i < 50 && !cond(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	waitFor(func() bool { return !strings.Contains(searches(ben), sid) })
	if strings.Contains(searches(ben), sid) || !strings.Contains(searches(anna), sid) {
		t.Fatal("after removing involved: Ben still sees it or Anna does not")
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/"+role+"/permissions/core.saved_search/read", "", nil); r.code != 204 {
		t.Fatalf("unset every scope: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodDelete, "/api/v1/roles/"+role+"/permissions/core.saved_search/read", "", nil); r.code != 404 {
		t.Fatalf("unset again: %d %s", r.code, r.raw)
	}
}
