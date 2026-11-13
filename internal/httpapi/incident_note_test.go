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
	"os"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD9 (IP28): the phone note is a form of the workplace, not of the portal.
// Sabine of the service desk takes a call of Anna: the incident has the channel phone, Anna as
// its requester (not Sabine), the number in its first entry, and the rule of the start data
// makes the callback a task of Sabine. Forms and rules refuse what cannot work.
func TestPhoneNoteOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd9"))
	api := apiServer(t)
	ctx := t.Context()
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 30}, {"../../packages/incident", 58}} {
		p := installPackage(t, ctx, api.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	role := "sd9_agent"
	dropRole(t, api, role)
	if r := call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD9"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{`{"type_name": "incident.incident", "action": "create", "scope": "all"}`,
		`{"type_name": "incident.incident", "action": "read", "scope": "all"}`,
		`{"type_name": "incident.task", "action": "read", "scope": "all"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	sabine := timelineLogin(t, api, "apitest_sd9", role, `{"first_name": "Sabine", "last_name": "SD9"}`)
	anna := create("core.person", `{"first_name": "Anna", "last_name": "SD9"}`)

	// The notes are offered in the workplace, not in the portal.
	names := func(place string) map[string]string {
		t.Helper()
		path := "/api/v1/forms"
		if place != "" {
			path += "?place=" + place
		}
		r := call(t, sabine.api, http.MethodGet, path, "", nil)
		if r.code != 200 {
			t.Fatalf("forms %s: %d %s", place, r.code, r.raw)
		}
		var list []map[string]any
		if err := json.Unmarshal([]byte(r.raw), &list); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, f := range list {
			out[f["name"].(string)] = f["id"].(string)
		}
		return out
	}
	work := names("workplace")
	if work["Telefonnotiz"] == "" || work["Vor-Ort-Notiz"] == "" {
		t.Fatalf("workplace: %v", work)
	}
	if portal := names(""); portal["Telefonnotiz"] != "" {
		t.Fatalf("the portal offers the phone note: %v", portal)
	}

	// Sabine takes Anna's call.
	r := call(t, sabine.api, http.MethodPost, "/api/v1/forms/"+work["Telefonnotiz"]+":submit", `{"answers": {"anforderer": "`+anna+`",
		"rueckrufnummer": "0211 939-4711", "titel": "SD9 Fachverfahren startet nicht", "auswirkung": "medium", "dringlichkeit": "low", "rueckruf": true}}`, nil)
	if r.code != 201 {
		t.Fatalf("submit: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	const inc = "/api/v1/records/incident.incident"
	got := call(t, api, http.MethodGet, inc+"/"+id, "", nil).body
	channel := call(t, api, http.MethodGet, "/api/v1/records/sd.channel/"+fmt.Sprint(got["channel"]), "", nil).body
	if channel["key"] != "phone" {
		t.Fatalf("channel: %v", channel)
	}
	requesters, _ := got["actors"].(map[string]any)["requester"].([]any)
	if len(requesters) != 1 || requesters[0].(map[string]any)["id"] != anna {
		t.Fatalf("requester: %v (Anna %s, Sabine %s)", got["actors"], anna, sabine.person)
	}
	if bodies := entryBodies(timeline(t, api, inc+"/"+id)); len(bodies) != 1 || !strings.Contains(bodies[0], "Rückrufnummer: 0211 939-4711") {
		t.Fatalf("first entry: %v", bodies)
	}

	// The rule makes the callback a task of Sabine; the incident counts it.
	var task map[string]any
	deadline := time.Now().Add(20 * time.Second)
	for task == nil {
		for _, d := range api.rt.Databases() {
			if err := api.ruleEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
		}
		page := call(t, sabine.api, http.MethodPost, "/api/v1/records/incident.task:query", `{"where": {"field": "incident", "eq": "`+id+`"}}`, nil)
		if items, _ := page.body["items"].([]any); len(items) == 1 {
			task = items[0].(map[string]any)
		} else if time.Now().After(deadline) {
			t.Fatalf("no callback task: %d %s", page.code, page.raw)
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}
	assignees, _ := task["actors"].(map[string]any)["assignee"].([]any)
	if task["title"] != "Rückruf" || task["state"] != "open" || len(assignees) != 1 || assignees[0].(map[string]any)["id"] != sabine.person {
		t.Fatalf("task: %v", task)
	}
	if n := call(t, api, http.MethodGet, inc+"/"+id, "", nil).body["tasks_open"]; n != float64(1) {
		t.Fatalf("open tasks: %v", n)
	}

	// Without the wish for a callback, no task.
	r = call(t, sabine.api, http.MethodPost, "/api/v1/forms/"+work["Vor-Ort-Notiz"]+":submit", `{"answers": {"anforderer": "`+anna+`", "titel": "SD9 Maus"}}`, nil)
	if r.code != 201 {
		t.Fatalf("on-site note: %d %s", r.code, r.raw)
	}
	if got := call(t, api, http.MethodGet, inc+"/"+r.body["id"].(string), "", nil).body; got["tasks_open"] != float64(0) {
		t.Fatalf("a task without a callback: %v", got["tasks_open"])
	}

	// Forms refuse a role with two types of holder, a key the list lacks and a field both
	// asked and fixed; a rule refuses @record where it does not fit.
	form := func(def string) string {
		return `{"name": "SD9 falsch", "target_type": "incident.incident", "place": "workplace", "definition": ` + fmt.Sprintf("%q", def) + `}`
	}
	ask := `{"id": "t", "field": "title"}`
	for def, want := range map[string]string{
		`{"sections": [{"id": "s", "title": {"de": "S"}, "questions": [` + ask + `, {"id": "o", "role": "observer"}]}]}`:                "one type of holder",
		`{"fixed": {"channel": "brieftaube"}, "sections": [{"id": "s", "title": {"de": "S"}, "questions": [` + ask + `]}]}`:             "no entry with the key",
		`{"fixed": {"title": "x"}, "sections": [{"id": "s", "title": {"de": "S"}, "questions": [` + ask + `]}]}`:                        "cannot be fixed too",
		`{"sections": [{"id": "s", "title": {"de": "S"}, "questions": [` + ask + `, {"id": "r", "role": "requester", "field": "x"}]}]}`: "only one of them",
	} {
		if r := call(t, api, http.MethodPost, "/api/v1/records/core.form", form(def), nil); r.code != 422 || !strings.Contains(r.raw, want) {
			t.Fatalf("%s: %d %s, want %s", def, r.code, r.raw, want)
		}
	}
	if r := call(t, api, http.MethodPut, "/api/v1/rules/sd9_wrong", `{"type_name": "incident.incident", "trigger": "create", "mode": "after",
		"actions": [{"create": {"type_name": "incident.task", "fields": {"title": "x", "incident": "@record", "description": "@record"}}}]}`, nil); r.code != 422 || !strings.Contains(r.raw, "@record fits only a reference") {
		t.Fatalf("rule: %d %s", r.code, r.raw)
	}
}
