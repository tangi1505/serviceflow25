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
	"slices"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD6 (SD-Plan E14): a request with a template of two stages asks both by the
// rule of the start data; after both agreed it is in fulfilment and has the two tasks of the
// template for their group, in their order; a refusal closes the request with its reason.
// Without a template nothing is asked; a template set later asks then. Rules refuse what
// cannot work.
func TestRequestOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd6"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 30}, {"../../packages/request", 21}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd6_user", "label": {"de": "SD6"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	lead := timelineLogin(t, admin, "apitest_sd6_lead", "sd6_user", `{"last_name": "Leitung"}`)
	sec := timelineLogin(t, admin, "apitest_sd6_sec", "sd6_user", `{"last_name": "Sicher"}`)
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, admin, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	leads := create("core.group", `{"name": "SD6 Leitung Nord"}`)
	create("core.group_member", `{"group": "`+leads+`", "person": "`+lead.person+`"}`)
	files := create("core.group", `{"name": "SD6 Fileserver"}`)
	other := create("core.person", `{"last_name": "Prüfer"}`)

	// The template: its approval template with two stages, the second "at least 1 of 2" (since
	// WF7 a core.approval_template instead of request.template_stage), and two task lines.
	approval := create("core.approval_template", `{"name": "Zugang Gruppenlaufwerk", "message": "Bitte genehmigen Sie diese Anfrage."}`)
	create("core.approval_template_stage", `{"template": "`+approval+`", "name": "IT-Sicherheit", "need": 1, "position": 20,
		"actors": {"approvers": [{"type": "core.person", "id": "`+sec.person+`"}, {"type": "core.person", "id": "`+other+`"}]}}`)
	create("core.approval_template_stage", `{"template": "`+approval+`", "name": "Fachliche Prüfung", "position": 10,
		"actors": {"approvers": [{"type": "core.group", "id": "`+leads+`"}]}}`)
	tpl := create("request.template", `{"name": "Zugang Gruppenlaufwerk", "approval_template": "`+approval+`"}`)
	create("request.template_task", `{"template": "`+tpl+`", "title": "Rechte setzen", "group": "`+files+`", "position": 20}`)
	create("request.template_task", `{"template": "`+tpl+`", "title": "Laufwerk anlegen", "group": "`+files+`", "position": 10}`)

	const requests = "/api/v1/records/request.request"
	get := func(id string) map[string]any {
		t.Helper()
		r := call(t, admin, http.MethodGet, requests+"/"+id, "", nil)
		if r.code != 200 {
			t.Fatalf("request: %d %s", r.code, r.raw)
		}
		return r.body
	}
	// settle hands the events to the rules until cond holds.
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
	approvals := func(id string) []map[string]any {
		t.Helper()
		r := call(t, admin, http.MethodGet, requests+"/"+id+"/approvals", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("approvals: %d %s", r.code, r.raw)
		}
		return list
	}
	decide := func(u timelineUser, id, body string) {
		t.Helper()
		list := approvals(id)
		if r := call(t, u.api, http.MethodPost, requests+"/"+id+"/approvals/"+list[0]["id"].(string)+":decide", body, nil); r.code != 200 {
			t.Fatalf("decide: %d %s", r.code, r.raw)
		}
	}

	// With the template: both stages asked in their order, the request waits.
	id := create("request.request", `{"title": "Zugang N:\\Nord für J. Neu", "template": "`+tpl+`"}`)
	settle("the approval of the template", func() bool { return get(id)["state"] == "approval" })
	list := approvals(id)
	stages := list[0]["stages"].([]any)
	first, second := stages[0].(map[string]any), stages[1].(map[string]any)
	if len(stages) != 2 || first["name"] != "Fachliche Prüfung" || second["name"] != "IT-Sicherheit" || second["need"] != float64(1) ||
		len(second["steps"].([]any)) != 2 || list[0]["message"] != "Bitte genehmigen Sie diese Anfrage." || list[0]["template"] != approval {
		t.Fatalf("approval from the template: %v", list)
	}
	// WF7: the first stage reminds after two days (no calendar, no setting), the second not yet.
	if first["remind_at"] == nil || second["remind_at"] != nil || first["reminders"] != float64(0) {
		t.Fatalf("reminders of the stages: %v", stages)
	}
	if rec := get(id); rec["approval"] != "pending" || rec["number"] == nil {
		t.Fatalf("request asked: %v", rec)
	}

	// Both agree: in fulfilment, the two tasks for their group in their order.
	decide(lead, id, `{"decision": "approve"}`)
	decide(sec, id, `{"decision": "approve", "comment": "ok"}`)
	if rec := get(id); rec["state"] != "fulfilment" || rec["approval"] != "approved" {
		t.Fatalf("after both stages: %v", rec)
	}
	var tasks []any
	settle("the tasks of the template", func() bool {
		r := call(t, admin, http.MethodPost, "/api/v1/records/request.task:query",
			`{"where": {"field": "request", "eq": "`+id+`"}, "sort": ["position"]}`, nil)
		tasks, _ = r.body["items"].([]any)
		return len(tasks) == 2
	})
	var titles []string
	for _, x := range tasks {
		task := x.(map[string]any)
		titles = append(titles, task["title"].(string))
		holders, _ := task["actors"].(map[string]any)["assignee"].([]any)
		if len(holders) != 1 || holders[0].(map[string]any)["id"] != files || task["state"] != "open" {
			t.Fatalf("task: %v", task)
		}
	}
	if !slices.Equal(titles, []string{"Laufwerk anlegen", "Rechte setzen"}) {
		t.Fatalf("tasks: %v", titles)
	}
	if n := get(id)["tasks_open"]; n != float64(2) {
		t.Fatalf("open tasks: %v", n)
	}
	// Waiting needs its reason.
	if r := call(t, admin, http.MethodPost, requests+"/"+id+":transition", `{"transition": "wait"}`, nil); r.code != 422 {
		t.Fatalf("waiting without a reason: %d %s", r.code, r.raw)
	}

	// A refusal closes the request with its reason.
	refused := create("request.request", `{"title": "Zugang N:\\Süd", "template": "`+tpl+`"}`)
	settle("the second approval", func() bool { return get(refused)["state"] == "approval" })
	decide(lead, refused, `{"decision": "reject", "comment": "Kein Bedarf"}`)
	if rec := get(refused); rec["state"] != "rejected" || rec["approval"] != "rejected" {
		t.Fatalf("after the refusal: %v", rec)
	}
	if bodies := entryBodies(timeline(t, admin, requests+"/"+refused)); len(bodies) != 1 || !strings.Contains(bodies[0], "Kein Bedarf") {
		t.Fatalf("the reason: %v", bodies)
	}

	// Without a template nothing is asked; set later, it asks.
	plain := create("request.request", `{"title": "Zweiter Bildschirm"}`)
	if r := call(t, admin, http.MethodPatch, requests+"/"+plain, `{"template": "`+tpl+`"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("set the template: %d %s", r.code, r.raw)
	}
	settle("the approval of the later template", func() bool { return get(plain)["state"] == "approval" })
	direct := create("request.request", `{"title": "Auskunft"}`)
	if r := call(t, admin, http.MethodPost, requests+"/"+direct+":transition", `{"transition": "start"}`, nil); r.code != 200 || r.body["state"] != "fulfilment" {
		t.Fatalf("start without a template: %d %s", r.code, r.raw)
	}
	if list := approvals(direct); len(list) != 0 {
		t.Fatalf("asked without a template: %v", list)
	}
	// Taken by hand only through an approval: "ask" without one is refused.
	if r := call(t, admin, http.MethodPost, requests+"/"+create("request.request", `{"title": "Ohne"}`)+":transition", `{"transition": "ask"}`, nil); r.code != 409 {
		t.Fatalf("ask without an approval: %d %s", r.code, r.raw)
	}

	// The tile of the portal.
	r := call(t, admin, http.MethodPost, "/api/v1/records/core.portal_tile:query", `{"limit": 50}`, nil)
	if !strings.Contains(r.raw, `"Etwas anfragen"`) {
		t.Fatalf("tiles: %s", r.raw)
	}

	// Rules refuse what cannot work.
	for _, body := range []string{
		`{"type_name": "probe.probe", "trigger": "create", "mode": "after", "actions": [{"approval": {"stages": {"type_name": "core.approval_template_stage", "match": {"template": "title"}}, "approvers": "approvers"}}]}`,
		`{"type_name": "request.request", "trigger": "create", "mode": "after", "actions": [{"approval": {"stages": {"type_name": "core.approval_template_stage", "match": {"nothing": "template"}}, "approvers": "approvers"}}]}`,
		`{"type_name": "request.request", "trigger": "create", "mode": "after", "actions": [{"approval": {"stages": {"type_name": "core.approval_template_stage", "match": {"template": "template"}}, "approvers": "nobody"}}]}`,
		`{"type_name": "request.request", "trigger": "create", "mode": "before", "actions": [{"approval": {"stages": {"type_name": "core.approval_template_stage", "match": {"template": "template"}}, "approvers": "approvers"}}]}`,
		`{"type_name": "request.request", "trigger": "create", "mode": "after", "actions": [{"approval": {"template": "title"}}]}`,
		`{"type_name": "request.request", "trigger": "create", "mode": "after", "actions": [{"approval": {"template": "template"}}]}`,
		`{"type_name": "request.request", "trigger": "create", "mode": "after", "actions": [{"approval": {"template": "template.approval_template", "role": "group"}}]}`,
		`{"type_name": "request.request", "trigger": "update", "mode": "after", "actions": [{"create": {"type_name": "request.task", "fields": {"request": "@record", "title": "@item.title"}, "each": {"type_name": "request.template_task", "match": {"template": "template"}, "order": "nothing"}}}]}`,
	} {
		if r := call(t, admin, http.MethodPut, "/api/v1/rules/sd6_wrong", body, nil); r.code != 422 {
			t.Fatalf("rule %s: %d %s", body, r.code, r.raw)
		}
	}
}
