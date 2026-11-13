// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of UI14a (UI-Plan E40): an incident created ten hours ago, waiting two of them,
// is resolved: "resolved at" is set, ten hours round the clock and eight in service time;
// reopened, the three fields are empty, resolved again they are set anew. A widget groups
// incidents by a role, one with two holders counts for both. One who only reads a shared
// dashboard copies it into one of their own, which they may change; the original stays.
func TestResolutionAndDashboardCopyOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_ui14a"))
	api := apiServer(t)
	ctx := t.Context()
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	for _, dir := range []string{"../../packages/sd", "../../packages/incident"} {
		p := installPackage(t, ctx, api.rt, dir)
		if _, err := starter.Package(ctx, p); err != nil {
			t.Fatalf("start data of %s: %v", p.Name, err)
		}
	}
	create := func(a *testAPI, typeName, body string) map[string]any {
		t.Helper()
		r := call(t, a, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body
	}
	const inc = "/api/v1/records/incident.incident"
	get := func(id string) resp { return call(t, api, http.MethodGet, inc+"/"+id, "", nil) }
	transition := func(id, body string) resp {
		t.Helper()
		v := get(id).body["version"]
		r := call(t, api, http.MethodPost, inc+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)})
		if r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
		return r
	}
	var kind string
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.solution_kind?limit=50", "", nil).body["items"].([]any) {
		kind = it.(map[string]any)["id"].(string)
	}
	resolve := `{"transition": "resolve", "fields": {"solution": "Neu gestartet", "solution_kind": "` + kind + `"}}`

	// Created ten hours ago, in progress from nine, waiting from six to four hours ago.
	id := create(api, "incident.incident", `{"title": "UI14a Drucker", "impact": "medium", "urgency": "medium"}`)["id"].(string)
	transition(id, `{"transition": "start"}`)
	reason := call(t, api, http.MethodGet, "/api/v1/records/core.wait_reason?limit=1", "", nil).body["items"].([]any)[0].(map[string]any)["id"].(string)
	transition(id, `{"transition": "wait", "fields": {"wait_reason": "`+reason+`"}}`)
	transition(id, `{"transition": "resume"}`)
	back := func(sql string, args ...any) {
		t.Helper()
		if _, err := api.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	back(`UPDATE rec_incident_incident SET created_at = now() - interval '10 hours' WHERE id = $1`, id)
	for to, ago := range map[string]string{"in_progress": "9 hours", "waiting": "6 hours"} {
		back(`UPDATE audit SET at = now() - $3::interval WHERE record_id = $1 AND field = 'state' AND new_value = to_jsonb($2::text) AND old_value <> '"waiting"'::jsonb`, id, to, ago)
	}
	back(`UPDATE audit SET at = now() - interval '4 hours' WHERE record_id = $1 AND field = 'state' AND old_value = '"waiting"'::jsonb`, id)

	near := func(got any, want float64) bool { n, ok := got.(float64); return ok && n > want-0.05 && n < want+0.05 }
	r := transition(id, resolve)
	if r.body["resolved_at"] == nil || !near(r.body["resolution_hours"], 10) || !near(r.body["resolution_service_hours"], 8) {
		t.Fatalf("resolved: %v %v %v", r.body["resolved_at"], r.body["resolution_hours"], r.body["resolution_service_hours"])
	}
	// The fields are the core's: a write may not set them.
	if r := call(t, api, http.MethodPatch, inc+"/"+id, `{"resolution_hours": 1}`, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, get(id).body["version"])}); r.code != 422 {
		t.Fatalf("written by hand: %d %s", r.code, r.raw)
	}
	r = transition(id, `{"transition": "reopen"}`)
	if r.body["resolved_at"] != nil || r.body["resolution_hours"] != nil || r.body["resolution_service_hours"] != nil {
		t.Fatalf("reopened: %s", r.raw)
	}
	r = transition(id, resolve)
	if r.body["resolved_at"] == nil || !near(r.body["resolution_hours"], 10) {
		t.Fatalf("resolved again: %s", r.raw)
	}
	// Closed is done as well: the resolution stays.
	if r = transition(id, `{"transition": "close"}`); r.body["resolved_at"] == nil {
		t.Fatalf("closed: %s", r.raw)
	}

	// Grouped by a role: observed by A and B, by A, by nobody.
	a := create(api, "core.group", `{"name": "UI14a A"}`)["id"].(string)
	b := create(api, "core.group", `{"name": "UI14a B"}`)["id"].(string)
	obs := func(ids ...string) string {
		var parts []string
		for _, g := range ids {
			parts = append(parts, `{"type": "core.group", "id": "`+g+`"}`)
		}
		return `"actors": {"observer": [` + strings.Join(parts, ",") + `]}`
	}
	create(api, "incident.incident", `{"title": "UI14a zwei", "impact": "low", "urgency": "low", `+obs(a, b)+`}`)
	create(api, "incident.incident", `{"title": "UI14a eine", "impact": "low", "urgency": "low", `+obs(a)+`}`)
	d := create(api, "core.dashboard", `{"name": "UI14a Statistik"}`)
	did := d["id"].(string)
	save := func(widgets string) resp {
		cur := call(t, api, http.MethodGet, "/api/v1/dashboards/"+did, "", nil).body["dashboard"].(map[string]any)
		return call(t, api, http.MethodPut, "/api/v1/dashboards/"+did, fmt.Sprintf(`{"version": %v, "widgets": [%s]}`, cur["version"], widgets), nil)
	}
	if r := save(`{"title": "Keine Rolle", "kind": "bar", "target_type": "incident.incident", "group_by": "actors.nope"}`); r.code != 422 || !strings.Contains(r.raw, "not a role") {
		t.Fatalf("unknown role: %d %s", r.code, r.raw)
	}
	r = save(`{"title": "Nach Beobachter", "kind": "bar", "target_type": "incident.incident", "group_by": "actors.observer"},
		{"title": "Dauer", "kind": "number", "target_type": "incident.incident", "measure": "avg", "measure_field": "resolution_service_hours", "x": 2}`)
	if r.code != 200 {
		t.Fatalf("save: %d %s", r.code, r.raw)
	}
	wid := map[string]string{}
	for _, w := range r.body["widgets"].([]any) {
		m := w.(map[string]any)
		wid[m["title"].(string)] = m["id"].(string)
	}
	data := call(t, api, http.MethodPost, "/api/v1/dashboards/"+did+"/widgets/"+wid["Nach Beobachter"]+"/data", `{}`, nil)
	counts := map[string]float64{}
	for _, p := range data.body["points"].([]any) {
		m := p.(map[string]any)
		label := "ohne"
		if l, ok := m["label"].(map[string]any); ok {
			label = l["de"].(string)
		}
		counts[label] = m["value"].(float64)
	}
	if counts["UI14a A"] != 2 || counts["UI14a B"] != 1 || counts["ohne"] != 1 {
		t.Fatalf("by observer: %v (%s)", counts, data.raw)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/dashboards/"+did+"/widgets/"+wid["Dauer"]+"/data", `{}`, nil); !near(r.body["value"], 8) {
		t.Fatalf("average service time: %s", r.raw)
	}

	// A reader of the shared dashboard copies it; without the right to create she may not.
	const role = "ui14a_reader"
	dropRole(t, api, role)
	call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "Leserin"}}`, nil)
	permit := func(p string) {
		t.Helper()
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	permit(`{"type_name": "core.dashboard", "action": "read", "scope": "involved"}`)
	permit(`{"type_name": "incident.incident", "action": "read", "scope": "all"}`)
	reader := timelineLogin(t, api, "apitest_ui14a_reader", role, `{"last_name": "Leserin"}`)
	cur := call(t, api, http.MethodGet, "/api/v1/records/core.dashboard/"+did, "", nil).body
	if r := call(t, api, http.MethodPatch, "/api/v1/records/core.dashboard/"+did, `{"actors": {"shared": [{"type": "core.person", "id": "`+reader.person+`"}]}}`,
		map[string]string{"If-Match": fmt.Sprintf(`"%v"`, cur["version"])}); r.code != 200 {
		t.Fatalf("share: %d %s", r.code, r.raw)
	}
	if r := call(t, reader.api, http.MethodGet, "/api/v1/dashboards/"+did, "", nil); r.code != 200 || r.body["editable"] != false {
		t.Fatalf("she reads it: %d %s", r.code, r.raw)
	}
	if r := call(t, reader.api, http.MethodPost, "/api/v1/dashboards/"+did+"/copy", `{"name": "Meine Statistik"}`, nil); r.code != 403 {
		t.Fatalf("copy without create: %d %s", r.code, r.raw)
	}
	permit(`{"type_name": "core.dashboard", "action": "create", "scope": "all"}`)
	permit(`{"type_name": "core.dashboard", "action": "update", "scope": "own"}`)
	cp := call(t, reader.api, http.MethodPost, "/api/v1/dashboards/"+did+"/copy", `{"name": "Meine Statistik"}`, nil)
	if cp.code != 201 || cp.body["editable"] != true || len(cp.body["widgets"].([]any)) != 2 {
		t.Fatalf("copy: %d %s", cp.code, cp.raw)
	}
	copied := cp.body["dashboard"].(map[string]any)
	if copied["name"] != "Meine Statistik" || copied["id"] == did {
		t.Fatalf("the copy: %v", copied)
	}
	// Hers to change; the original keeps its two widgets and stays not hers to change.
	if r := call(t, reader.api, http.MethodPut, "/api/v1/dashboards/"+copied["id"].(string), fmt.Sprintf(`{"version": %v, "widgets": []}`, copied["version"]), nil); r.code != 200 {
		t.Fatalf("change the copy: %d %s", r.code, r.raw)
	}
	if r := call(t, reader.api, http.MethodGet, "/api/v1/dashboards/"+did, "", nil); len(r.body["widgets"].([]any)) != 2 || r.body["editable"] != false {
		t.Fatalf("the original: %s", r.raw)
	}
}
