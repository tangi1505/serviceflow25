// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/query"
)

// secondInstance is another api instance on the same database and Valkey, with its own
// rights service and cache: what a second pod in Kubernetes is.
func secondInstance(t *testing.T, first *testAPI) *testAPI {
	t.Helper()
	az := authz.New(db.Single(first.pool), first.kv, quiet)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = az.Run(ctx) }()
	h := NewRouter(Options{Logger: quiet, Version: "test", API: true, Records: withHook(first.rt, az), Auth: first.auth,
		Org: org.New(db.Single(first.pool)), Groups: query.NewGroups(db.Single(first.pool)), Authz: az})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &testAPI{Server: srv, cookie: first.cookie, auth: first.auth, authz: az, pool: first.pool, kv: first.kv}
}

// The Prüfweg of PL9: a field without read right is missing in the API answer, in the type
// listing and in the OpenAPI; a field without write right is refused; a condition in CEL
// narrows the list; after a revoke on one instance the other denies within a second.
func TestFieldRightsConditionsAndInvalidation(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"

	// Role: probe.probe read only title and kind, write only title, only faults (condition).
	dropRole(t, admin, "rights_fields")
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "rights_fields"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	perm := `{"type_name": "probe.probe", "action": "%s", "scope": "all", "condition": "record.kind == \"fault\"", "fields": {"read": ["title", "kind"], "write": ["title"]}}`
	for _, action := range []string{"read", "update"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/rights_fields/permissions", strings.ReplaceAll(perm, "%s", action), nil); r.code != 204 {
			t.Fatalf("permission %s: %d %s", action, r.code, r.raw)
		}
	}
	bad := `{"type_name": "probe.probe", "action": "read", "scope": "all", "condition": "record.title.startsWith(\"x\")"}`
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/rights_fields/permissions", bad, nil); r.code != 422 {
		t.Fatalf("untranslatable condition: %d %s", r.code, r.raw)
	}

	// A person with that role, signed in.
	acc, err := admin.auth.CreateAccount(ctx, "apitest_fields", "apitest_fields", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "apitest_fields"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	assign := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "rights_fields", "person_id": "`+pid.String()+`"}`, nil)
	if assign.code != 201 {
		t.Fatalf("assign: %d %s", assign.code, assign.raw)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_fields", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr), auth: admin.auth, authz: admin.authz, pool: admin.pool, kv: admin.kv}

	// Records: a fault and a question (the condition hides the question).
	fault := call(t, admin, http.MethodPost, base, `{"title": "fields fault", "kind": "fault", "priority": 4}`, nil)
	question := call(t, admin, http.MethodPost, base, `{"title": "fields question", "kind": "question"}`, nil)
	if fault.code != 201 || question.code != 201 {
		t.Fatalf("create: %d %d", fault.code, question.code)
	}
	fid := fault.body["id"].(string)

	// Read: only title and kind come back, priority is missing; the question is invisible.
	r := call(t, user, http.MethodGet, base+"/"+fid, "", nil)
	if r.code != 200 || r.body["title"] != "fields fault" || r.body["kind"] != "fault" {
		t.Fatalf("read fault: %d %s", r.code, r.raw)
	}
	if _, has := r.body["priority"]; has {
		t.Fatalf("priority must be hidden: %s", r.raw)
	}
	if _, has := r.body["id"]; !has {
		t.Fatalf("system fields stay: %s", r.raw)
	}
	if r := call(t, user, http.MethodGet, base+"/"+question.body["id"].(string), "", nil); r.code != 404 {
		t.Fatalf("question hidden by the condition: %d %s", r.code, r.raw)
	}
	r = call(t, user, http.MethodGet, base+"?filter[title]=fields%20question&count=true", "", nil)
	var page struct {
		Items []map[string]any `json:"items"`
		Total *int64           `json:"total"`
	}
	if err := json.Unmarshal([]byte(r.raw), &page); err != nil || len(page.Items) != 0 || page.Total == nil || *page.Total != 0 {
		t.Fatalf("list hides the question: %d %s", r.code, r.raw)
	}
	r = call(t, user, http.MethodGet, base+"?filter[title]=fields%20fault", "", nil)
	_ = json.Unmarshal([]byte(r.raw), &page)
	if len(page.Items) != 1 {
		t.Fatalf("list shows the fault: %s", r.raw)
	}
	if _, has := page.Items[0]["priority"]; has {
		t.Fatalf("priority must be hidden in the list: %s", r.raw)
	}

	// Types and OpenAPI as the person sees them.
	r = call(t, user, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
	var typ struct {
		Fields []map[string]any `json:"fields"`
	}
	_ = json.Unmarshal([]byte(r.raw), &typ)
	if len(typ.Fields) != 2 || typ.Fields[0]["name"] != "title" || typ.Fields[1]["name"] != "kind" {
		t.Fatalf("visible fields of the type: %s", r.raw)
	}
	// The list of types holds only what the person may read, each with a field array
	// (a type without readable fields once came as null and broke the interface).
	r = call(t, user, http.MethodGet, "/api/v1/types", "", nil)
	var typeList []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &typeList)
	// Select lists are readable by everyone (PL16), so only the other types count.
	var plain []string
	for _, ty := range typeList {
		isList := false
		if traits, _ := ty["traits"].([]any); traits != nil {
			for _, tr := range traits {
				if tr == "list" {
					isList = true
				}
			}
		}
		if !isList {
			plain = append(plain, ty["name"].(string))
		}
	}
	if len(plain) != 1 || plain[0] != "probe" || strings.Contains(r.raw, `"fields":null`) {
		t.Fatalf("types the person may read: %s", r.raw)
	}
	r = call(t, user, http.MethodGet, "/api/v1/openapi.json", "", nil)
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	_ = json.Unmarshal([]byte(r.raw), &spec)
	props := spec.Components.Schemas["probe.probe"].Properties
	if _, has := props["priority"]; has || props["kind"] == nil {
		t.Fatalf("openapi must omit priority for this person: %v", props)
	}

	// Write: title may change, priority may not.
	if r := call(t, user, http.MethodPatch, base+"/"+fid, `{"title": "fields fault 2"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("write title: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodPatch, base+"/"+fid, `{"priority": 1}`, map[string]string{"If-Match": `"2"`}); r.code != 403 {
		t.Fatalf("write priority: %d %s", r.code, r.raw)
	}

	// Two instances: revoke on the second, the first denies within a second.
	second := secondInstance(t, admin)
	userOnSecond := &testAPI{Server: second.Server, cookie: user.cookie}
	if r := call(t, userOnSecond, http.MethodGet, base+"/"+fid, "", nil); r.code != 200 {
		t.Fatalf("second instance before revoke: %d %s", r.code, r.raw)
	}
	if r := call(t, user, http.MethodGet, base+"/"+fid, "", nil); r.code != 200 {
		t.Fatalf("first instance before revoke: %d", r.code)
	}
	adminOnSecond := &testAPI{Server: second.Server, cookie: admin.cookie}
	if r := call(t, adminOnSecond, http.MethodDelete, "/api/v1/assignments/"+assign.body["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("revoke on second: %d %s", r.code, r.raw)
	}
	deadline := time.Now().Add(time.Second)
	for {
		r := call(t, user, http.MethodGet, base+"/"+fid, "", nil)
		if r.code == 404 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first instance still allows after a second: %d", r.code)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if r := call(t, userOnSecond, http.MethodGet, base+"/"+fid, "", nil); r.code != 404 {
		t.Fatalf("second instance after revoke: %d", r.code)
	}
}
