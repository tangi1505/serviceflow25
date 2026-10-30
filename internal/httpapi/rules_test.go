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

	"group24.de/itsm/internal/records"
)

// The Prüfweg of WF4: a rule "kind fault → group Netz" assigns the group when the record is
// created, in the same answer; a rule after the write runs as a job and leaves a notification,
// and its write fires the next rule; a rule on a schedule finds its records; the history names
// the rules; wrong rules are refused.
func TestRulesOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	title := "WF4 " + suffix

	group := call(t, api, http.MethodPost, "/api/v1/records/core.group", `{"name": "WF4 Netz `+suffix+`"}`, nil)
	if group.code != 201 {
		t.Fatalf("group: %d %s", group.code, group.raw)
	}
	gid := group.body["id"].(string)
	put := func(name, body string) resp {
		t.Helper()
		return call(t, api, http.MethodPut, "/api/v1/rules/"+name, body, nil)
	}
	// Rules of an earlier run that stopped half-way go first; at the end the same again.
	removeRules := func() {
		for _, name := range []string{"wf4_netz", "wf4_follow", "wf4_chain", "wf4_time"} {
			call(t, api, http.MethodDelete, "/api/v1/rules/"+name, "", nil)
		}
	}
	removeRules()

	// Before the write: fault → group Netz, priority 2.
	r := put("wf4_netz", `{"label": {"de": "Störung an Netz", "en": "Fault to Netz"}, "type_name": "probe.probe",
		"trigger": "create", "mode": "before", "condition": "record.kind == \"fault\" && record.title.startsWith(\"`+title+`\")",
		"actions": [{"assign": {"role": "assigned", "to": [{"type": "core.group", "id": "`+gid+`"}]}}, {"set": {"priority": 2}}]}`)
	if r.code != 201 || r.body["enabled"] != true || r.body["version"] != float64(1) {
		t.Fatalf("rule before: %d %s", r.code, r.raw)
	}
	// After the write: a record with an assigned group gets a tag and a notification …
	if r = put("wf4_follow", `{"type_name": "probe.probe", "trigger": "create", "mode": "after",
		"condition": "size(record.actors.assigned) > 0 && record.title.startsWith(\"`+title+`\")",
		"actions": [{"set": {"tag_no": "NETZ"}}, {"notify": {"to": ["actors.assigned"], "message": "Neue Störung für Netz"}}]}`); r.code != 201 {
		t.Fatalf("rule after: %d %s", r.code, r.raw)
	}
	// … and that write (an update) fires the next rule.
	if r = put("wf4_chain", `{"type_name": "probe.probe", "trigger": "update", "mode": "after",
		"condition": "\"tag_no\" in changed && record.tag_no == \"NETZ\"", "actions": [{"set": {"due_on": "2026-12-01"}}]}`); r.code != 201 {
		t.Fatalf("rule chain: %d %s", r.code, r.raw)
	}

	// Created as a fault: the group and the priority are in the answer of the create.
	rec := call(t, api, http.MethodPost, base, `{"title": "`+title+` Router", "kind": "fault"}`, nil)
	if rec.code != 201 || rec.body["priority"] != float64(2) {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	actors, _ := rec.body["actors"].(map[string]any)
	if assigned, _ := actors["assigned"].([]any); len(assigned) != 1 || !strings.Contains(rec.raw, gid) {
		t.Fatalf("group in the answer: %s", rec.raw)
	}
	id := rec.body["id"].(string)
	// A wish is no fault: no rule before.
	wish := call(t, api, http.MethodPost, base, `{"title": "`+title+` Wunsch", "kind": "wish"}`, nil)
	if wish.code != 201 || strings.Contains(wish.raw, gid) {
		t.Fatalf("wish: %d %s", wish.code, wish.raw)
	}

	// The rules after the write run as jobs: tag, notification, then the chain.
	deadline := time.Now().Add(20 * time.Second)
	var got resp
	for {
		if err := api.ruleEvents.Dispatch(ctx); err != nil {
			t.Fatal(err)
		}
		got = call(t, api, http.MethodGet, base+"/"+id, "", nil)
		if got.body["tag_no"] == "NETZ" && got.body["due_on"] == "2026-12-01" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the rules after the write did not run: %s", got.raw)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if got.body["version"] != float64(3) {
		t.Fatalf("one version per rule after the write: %s", got.raw)
	}

	// The history names the rule of every change and the notification.
	h := call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil)
	var hist []map[string]any
	_ = json.Unmarshal([]byte(h.raw), &hist)
	byField := map[string]string{}
	notified := false
	for _, e := range hist {
		field, _ := e["field"].(string)
		rule, _ := e["rule"].(string)
		byField[field] = rule
		if e["action"] == "notify" && rule == "wf4_follow" && strings.Contains(h.raw, "Neue Störung für Netz") {
			notified = true
		}
	}
	if byField["actors.assigned"] != "wf4_netz" || byField["priority"] != "wf4_netz" || byField["tag_no"] != "wf4_follow" ||
		byField["due_on"] != "wf4_chain" || byField["title"] != "" || !notified {
		t.Fatalf("history: %v (%s)", byField, h.raw)
	}

	// A chain at the depth limit queues nothing.
	var before int
	_ = api.pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE queue = $1", ruleTestQueue).Scan(&before)
	if err := api.rules.Handle(ctx, records.Event{Kind: "update", TypeName: "probe.probe", RecordID: uuid.MustParse(id), Position: 1,
		Payload: json.RawMessage(`{"changed": ["tag_no"], "rule": "wf4_follow", "depth": 5}`)}); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = api.pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE queue = $1", ruleTestQueue).Scan(&after)
	if after != before {
		t.Fatalf("a chain at depth 5 must stop: %d jobs before, %d after", before, after)
	}

	// On a schedule: the condition finds the records in SQL.
	if r = put("wf4_time", `{"type_name": "probe.probe", "trigger": "time", "mode": "after", "schedule": "* * * * *",
		"condition": "record.title == \"`+title+` Wunsch\"", "actions": [{"set": {"priority": 5}}]}`); r.code != 201 {
		t.Fatalf("rule on a schedule: %d %s", r.code, r.raw)
	}
	ran, err := api.rules.RunSchedules(ctx, time.Now().Add(2*time.Minute))
	if err != nil || ran["wf4_time"] != 1 {
		t.Fatalf("scheduled run: %v %v", ran, err)
	}
	if w := call(t, api, http.MethodGet, base+"/"+wish.body["id"].(string), "", nil); w.body["priority"] != float64(5) {
		t.Fatalf("the scheduled rule set the priority: %s", w.raw)
	}

	// Wrong rules are refused with every problem; reading and replacing work.
	bad := put("wf4_bad", `{"type_name": "probe.probe", "trigger": "create", "mode": "before", "condition": "record.kind ==",
		"actions": [{"notify": {"to": ["actors.nobody"], "message": "x"}}, {"set": {"nofield": 1}}, {}]}`)
	for _, want := range []string{`"condition"`, `before the write a rule can only set and assign`, `nofield`, `exactly one of`} {
		if bad.code != 422 || !strings.Contains(bad.raw, want) {
			t.Fatalf("bad rule (%s): %d %s", want, bad.code, bad.raw)
		}
	}
	if bad = put("wf4_bad", `{"type_name": "probe.probe", "trigger": "time", "mode": "after", "schedule": "",
		"condition": "\"x\" in changed", "actions": [{"set": {"priority": 1}}]}`); bad.code != 422 || !strings.Contains(bad.raw, `"schedule"`) {
		t.Fatalf("time rule without schedule: %d %s", bad.code, bad.raw)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/rules/wf4_netz", "", nil); r.code != 200 || r.body["mode"] != "before" {
		t.Fatalf("get: %d %s", r.code, r.raw)
	}
	if r = put("wf4_netz", `{"type_name": "probe.probe", "trigger": "create", "mode": "before", "enabled": false,
		"actions": [{"set": {"priority": 2}}]}`); r.code != 200 || r.body["version"] != float64(2) || r.body["enabled"] != false {
		t.Fatalf("replace: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/rules", "", nil); r.code != 200 || !strings.Contains(r.raw, `"wf4_chain"`) {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodGet, "/api/v1/rules/wf4_none", "", nil); r.code != 404 {
		t.Fatalf("unknown rule: %d %s", r.code, r.raw)
	}

	removeRules()
	for _, rid := range []string{id, wish.body["id"].(string)} {
		call(t, api, http.MethodDelete, base+"/"+rid, "", nil)
		call(t, api, http.MethodPost, base+"/"+rid+":purge", "", nil)
	}
}

// PL41 over HTTP: a rule that fails for good when it runs (the record refuses its value)
// shows up in the failures with the rule, the record and the reason; the consumers show where
// they stand. A user without read on core.job sees neither.
func TestDeliveryFailuresOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	title := "PL41 " + suffix
	call(t, api, http.MethodDelete, "/api/v1/rules/pl41_clear", "", nil)
	// Valid when saved; at run time a resolved record needs its solution (WF2), so it fails.
	r := call(t, api, http.MethodPut, "/api/v1/rules/pl41_clear", `{"type_name": "probe.probe", "trigger": "update", "mode": "after",
		"condition": "record.state == \"resolved\" && record.title == \"`+title+`\"", "actions": [{"set": {"solution": null}}]}`, nil)
	if r.code != 201 {
		t.Fatalf("rule: %d %s", r.code, r.raw)
	}
	rec := call(t, api, http.MethodPost, base, `{"title": "`+title+`"}`, nil)
	id := rec.body["id"].(string)
	call(t, api, http.MethodPost, base+"/"+id+":transition", `{"transition": "start"}`, nil)
	if r = call(t, api, http.MethodPost, base+"/"+id+":transition", `{"transition": "resolve", "fields": {"solution": "x"}}`, map[string]string{"If-Match": `"2"`}); r.code != 200 {
		t.Fatalf("resolve: %d %s", r.code, r.raw)
	}

	var failure map[string]any
	deadline := time.Now().Add(20 * time.Second)
	for failure == nil {
		if err := api.ruleEvents.Dispatch(ctx); err != nil {
			t.Fatal(err)
		}
		r = call(t, api, http.MethodGet, "/api/v1/events/failures?limit=50", "", nil)
		var list []map[string]any
		_ = json.Unmarshal([]byte(r.raw), &list)
		for _, f := range list {
			if f["target"] == "pl41_clear" && f["record_id"] == id {
				failure = f
			}
		}
		if failure == nil && time.Now().After(deadline) {
			t.Fatalf("no failure of the rule: %d %s", r.code, r.raw)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if failure["kind"] != "rule" || failure["type_name"] != "probe.probe" || !strings.Contains(failure["error"].(string), "solution") {
		t.Fatalf("failure: %v", failure)
	}
	if got := call(t, api, http.MethodGet, base+"/"+id, "", nil); got.body["solution"] != "x" {
		t.Fatalf("the refused rule changed nothing: %s", got.raw)
	}

	r = call(t, api, http.MethodGet, "/api/v1/events/consumers", "", nil)
	if r.code != 200 || !strings.Contains(r.raw, `"name":"apitest-rules"`) || !strings.Contains(r.raw, `"lag":`) {
		t.Fatalf("consumers: %d %s", r.code, r.raw)
	}
	// Without read on core.job: 403.
	if _, err := api.auth.CreateAccount(ctx, "apitest_pl41_"+suffix, "PL41", testPassword, nil); err != nil {
		t.Fatal(err)
	}
	in := call(t, api, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_pl41_`+suffix+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	plain := &testAPI{Server: api.Server, cookie: sessionCookieOf(t, in.hdr)}
	for _, path := range []string{"/api/v1/events/consumers", "/api/v1/events/failures"} {
		if r = call(t, plain, http.MethodGet, path, "", nil); r.code != 403 {
			t.Fatalf("%s without the right: %d %s", path, r.code, r.raw)
		}
	}

	call(t, api, http.MethodDelete, "/api/v1/rules/pl41_clear", "", nil)
	call(t, api, http.MethodDelete, base+"/"+id, "", nil)
	call(t, api, http.MethodPost, base+"/"+id+":purge", "", nil)
}
