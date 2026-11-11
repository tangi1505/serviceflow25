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
	"os"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD17 (SD-Plan E16, N22): a new article gets the group "Wissensredaktion" as its
// editors; submitted, the editors are asked by the rule of the start data, their yes publishes
// it. Fassung 2 (a change of title and text) stays invisible to readers until the editors
// approved it, then it is the new version; a change of the category goes at once; while a
// change waits, title and text cannot change again; a refused change is dropped, a withdrawn
// one too, and the author may change again.
func TestKnowledgeApprovalOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd17"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 28}, {"../../packages/knowledge", 9}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	const arts = "/api/v1/records/knowledge.article"
	for _, role := range []string{"sd17_author", "sd17_user"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD17"}}`, nil); r.code != 201 {
			t.Fatalf("role: %d %s", r.code, r.raw)
		}
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/roles/sd17_author/permissions", `{"type_name": "knowledge.article", "action": "*", "scope": "all"}`, nil); r.code != 204 {
		t.Fatalf("permission: %d %s", r.code, r.raw)
	}
	author := timelineLogin(t, admin, "apitest_sd17_author", "sd17_author", `{"last_name": "Autorin"}`)
	editor := timelineLogin(t, admin, "apitest_sd17_editor", "sd17_user", `{"last_name": "Redaktion"}`)
	reader := timelineLogin(t, admin, "apitest_sd17_reader", "sd17_user", `{"last_name": "Leser"}`)
	q := call(t, admin, http.MethodPost, "/api/v1/records/core.group:query", `{"where": {"field": "name", "eq": "Wissensredaktion"}}`, nil)
	groups, _ := q.body["items"].([]any)
	if len(groups) != 1 {
		t.Fatalf("the group of the start data: %s", q.raw)
	}
	editors := groups[0].(map[string]any)["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+editors+`", "person": "`+editor.person+`"}`, nil); r.code != 201 {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}
	settle := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("waited in vain for %s", what)
			}
			for _, d := range admin.rt.Databases() {
				if err := admin.ruleEvents.Dispatch(d.Bind(ctx)); err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	get := func(api *testAPI, id string) resp {
		t.Helper()
		return call(t, api, http.MethodGet, arts+"/"+id, "", nil)
	}
	approvals := func(id string) []map[string]any {
		t.Helper()
		r := call(t, author.api, http.MethodGet, arts+"/"+id+"/approvals", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("approvals: %d %s", r.code, r.raw)
		}
		return list
	}
	decide := func(id, body string) {
		t.Helper()
		list := approvals(id)
		if r := call(t, editor.api, http.MethodPost, arts+"/"+id+"/approvals/"+list[0]["id"].(string)+":decide", body, nil); r.code != 200 {
			t.Fatalf("decide: %d %s", r.code, r.raw)
		}
	}
	patch := func(id, body string, want int) resp {
		t.Helper()
		r := call(t, author.api, http.MethodPatch, arts+"/"+id, body, map[string]string{"If-Match": versionOf(t, author.api, arts+"/"+id)})
		if r.code != want {
			t.Fatalf("change %s: %d %s, want %d", body, r.code, r.raw, want)
		}
		return r
	}

	// A new article: the editors are the group of the start data; publishing by hand is gone.
	r := call(t, author.api, http.MethodPost, arts, `{"title": "VPN verbindet nicht", "body": "<p>Starten Sie den Client neu.</p>", "audience": "all"}`, nil)
	if r.code != 201 {
		t.Fatalf("article: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	if eds, _ := r.body["actors"].(map[string]any)["editors"].([]any); len(eds) != 1 || eds[0].(map[string]any)["id"] != editors {
		t.Fatalf("editors: %v", r.body["actors"])
	}
	if r := call(t, author.api, http.MethodPost, arts+"/"+id+":transition", `{"transition": "publish"}`, nil); r.code != 409 {
		t.Fatalf("publish by hand: %d %s", r.code, r.raw)
	}

	// Submitted: the editors are asked; their yes publishes it.
	if r := call(t, author.api, http.MethodPost, arts+"/"+id+":transition", `{"transition": "submit"}`, nil); r.code != 200 || r.body["state"] != "review" {
		t.Fatalf("submit: %d %s", r.code, r.raw)
	}
	settle("the editors asked", func() bool { return get(admin, id).body["approval"] == "pending" })
	if r := get(reader.api, id); r.code != 404 {
		t.Fatalf("the reader sees the article in review: %d", r.code)
	}
	decide(id, `{"decision": "approve"}`)
	if r := get(reader.api, id); r.code != 200 || r.body["title"] != "VPN verbindet nicht" || r.body["state"] != "published" {
		t.Fatalf("published: %d %s", r.code, r.raw)
	}

	// Fassung 2: title and text wait, the category goes at once; the reader sees the old text.
	cat := listEntry(t, admin, "sd.category", "vpn")
	r = patch(id, `{"title": "VPN verbindet nicht oder bricht ab", "body": "<p>Neu anmelden.</p>", "category": "`+cat+`"}`, 200)
	if r.body["title"] != "VPN verbindet nicht" || r.body["category"] != cat || r.body["approval"] != "pending" {
		t.Fatalf("after the change: %v", r.body)
	}
	version := r.body["version"]
	list := approvals(id)
	changes, _ := list[0]["changes"].(map[string]any)
	if list[0]["state"] != "open" || changes["title"] != "VPN verbindet nicht oder bricht ab" || changes["body"] != "<p>Neu anmelden.</p>" || changes["category"] != nil {
		t.Fatalf("the change that waits: %v", list[0])
	}
	if r := get(reader.api, id); r.body["title"] != "VPN verbindet nicht" || strings.Contains(r.raw, "Neu anmelden") {
		t.Fatalf("the reader sees Fassung 2 before the approval: %s", r.raw)
	}
	// While it waits, title and text are not changed again; other fields are.
	patch(id, `{"title": "Noch einmal"}`, 409)
	patch(id, `{"expires_on": "2030-01-01"}`, 200)

	// Approved: the change is the new version, the reader sees it, the state stays.
	decide(id, `{"decision": "approve", "comment": "passt"}`)
	r = get(reader.api, id)
	if r.body["title"] != "VPN verbindet nicht oder bricht ab" || !strings.Contains(r.raw, "Neu anmelden") || r.body["state"] != "published" {
		t.Fatalf("after the approval: %s", r.raw)
	}
	if v := get(admin, id).body["version"]; v.(float64) <= version.(float64) {
		t.Fatalf("no new version: %v after %v", v, version)
	}

	// Refused: the change is dropped. Withdrawn: dropped too, and the author changes again.
	patch(id, `{"title": "Falsch"}`, 200)
	decide(id, `{"decision": "reject", "comment": "Titel zu kurz"}`)
	if r := get(reader.api, id); r.body["title"] != "VPN verbindet nicht oder bricht ab" {
		t.Fatalf("after the refusal: %v", r.body["title"])
	}
	patch(id, `{"audience": "agents"}`, 200)
	list = approvals(id)
	if r := call(t, author.api, http.MethodPost, arts+"/"+id+"/approvals/"+list[0]["id"].(string)+":withdraw", "", nil); r.code != 200 {
		t.Fatalf("withdraw: %d %s", r.code, r.raw)
	}
	if r := get(reader.api, id); r.code != 200 {
		t.Fatalf("after the withdrawal the reader still reads: %d", r.code)
	}
	patch(id, `{"title": "VPN: erste Schritte"}`, 200)
	if list := approvals(id); list[0]["state"] != "open" {
		t.Fatalf("a new change waits: %v", list[0])
	}

	// The block hold names fields of the type and a role for persons or groups.
	ty := call(t, admin, http.MethodGet, "/api/v1/types/knowledge.article", "", nil)
	if !strings.Contains(ty.raw, `"hold":{"fields":["title","body","audience"]`) {
		t.Fatalf("type: %.600s", ty.raw)
	}
}
