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
	"reflect"
	"sort"
	"testing"
	"time"

	"group24.de/itsm/internal/dashboard"
	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/records"
)

// The Prüfweg of PL69 (Hauptplan E195, Kern-Plan E73): after the count of the night, long
// periods read the day values and show what counting shows; a write raises the day value once
// the run of the minute counted its day; a person who reads her own records counts as before.
func TestDashboardDayValuesOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl69"))
	api := apiServer(t)
	ctx := t.Context()
	create := func(a *testAPI, body string) {
		t.Helper()
		if r := call(t, a, http.MethodPost, "/api/v1/records/probe.probe", body, nil); r.code != 201 {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
	}
	for i, kind := range []string{"fault", "fault", "question", "wish"} {
		create(api, fmt.Sprintf(`{"title": "PL69 %d", "kind": "%s", "priority": %d}`, i, kind, i+1))
	}
	const role = "pl69_viewer"
	dropRole(t, api, role)
	call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "Betrachter"}}`, nil)
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "own"}`, `{"type_name": "probe.probe", "action": "create", "scope": "all"}`,
		`{"type_name": "core.dashboard", "action": "read", "scope": "involved"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	viewer := timelineLogin(t, api, "apitest_pl69_viewer", role, `{"last_name": "Betrachterin"}`)
	create(viewer.api, `{"title": "PL69 ihrer", "kind": "wish", "priority": 2}`)
	d := call(t, api, http.MethodPost, "/api/v1/records/core.dashboard", `{"name": "PL69", "actors": {"shared": [{"type": "core.person", "id": "`+viewer.person+`"}]}}`, nil).body
	path := "/api/v1/dashboards/" + d["id"].(string)
	r := call(t, api, http.MethodPut, path, fmt.Sprintf(`{"version": %v, "widgets": [
		{"title": "Alle", "kind": "number", "target_type": "probe.probe", "x": 0, "y": 0},
		{"title": "Summe", "kind": "number", "target_type": "probe.probe", "measure": "sum", "measure_field": "priority", "x": 1, "y": 0},
		{"title": "Mittel", "kind": "number", "target_type": "probe.probe", "measure": "avg", "measure_field": "priority", "x": 2, "y": 0},
		{"title": "Nach Art", "kind": "pie", "target_type": "probe.probe", "group_by": "kind", "x": 0, "y": 1, "w": 2},
		{"title": "Je Woche", "kind": "line", "target_type": "probe.probe", "group_by": "created_at:week", "x": 2, "y": 1, "w": 2},
		{"title": "Offene", "kind": "number", "target_type": "probe.probe", "query": "{\"field\": \"state\", \"neq\": \"done\"}", "x": 3, "y": 0}]}`, d["version"]), nil)
	if r.code != 200 {
		t.Fatalf("save: %d %s", r.code, r.raw)
	}
	ids := map[string]string{}
	for _, w := range r.body["widgets"].([]any) {
		m := w.(map[string]any)
		ids[m["title"].(string)] = m["id"].(string)
	}
	data := func(a *testAPI, title, filters string) map[string]any {
		t.Helper()
		r := call(t, a, http.MethodPost, path+"/widgets/"+ids[title]+"/data", filters, nil)
		if r.code != 200 {
			t.Fatalf("data %s: %d %s", title, r.code, r.raw)
		}
		delete(r.body, "computed_at")
		if pts, ok := r.body["points"].([]any); ok {
			sort.Slice(pts, func(i, j int) bool { return fmt.Sprint(pts[i]) < fmt.Sprint(pts[j]) })
		}
		return r.body
	}
	year := fmt.Sprintf(`{"from": "%s"}`, time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.FixedZone("CET", 3600)).Format(time.RFC3339))
	periods := []string{`{}`, year}
	titles := []string{"Alle", "Summe", "Mittel", "Nach Art", "Je Woche", "Offene"}
	counted := map[string]map[string]any{}
	for _, p := range periods {
		for _, title := range titles {
			counted[p+title] = data(api, title, p)
		}
	}

	// The night's count; what it holds follows the widgets: kind as a group, priority as a
	// number, the open ones with their condition not at all.
	if n, err := api.dashboards.FillDays(ctx); err != nil || n != 1 {
		t.Fatalf("fill: %d %v", n, err)
	}
	var dims, measures []string
	if err := api.pool.QueryRow(ctx, "SELECT dims, measures FROM day_value_state WHERE type_name = 'probe.probe' AND date_field = 'created_at'").Scan(&dims, &measures); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(dims) != "[ kind]" || fmt.Sprint(measures) != "[priority]" {
		t.Fatalf("state: %v %v", dims, measures)
	}
	for _, p := range periods {
		for _, title := range titles {
			if got := data(api, title, p); !reflect.DeepEqual(got, counted[p+title]) {
				g, _ := json.Marshal(got)
				c, _ := json.Marshal(counted[p+title])
				t.Fatalf("%s %s: day values %s, counted %s", title, p, g, c)
			}
		}
	}

	// A write: the day values show it once the run of the minute counted the marked day; a
	// count shows it at once. That the two differ in between shows where the number comes from.
	days := events.New(api.rt, records.New(api.rt), nil, quiet)
	days.Prefix = "apitest-pl69-"
	days.Consume(dashboard.ConsumerDays, api.dashboards.MarkDays)
	for _, db := range api.rt.Databases() {
		if err := days.SkipToEnd(db.Bind(ctx)); err != nil {
			t.Fatal(err)
		}
	}
	create(api, `{"title": "PL69 neu", "kind": "fault", "priority": 5}`)
	if v := data(api, "Alle", `{}`)["value"]; v != 5.0 {
		t.Fatalf("before the run of the minute: %v", v)
	}
	if err := days.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := api.dashboards.CountDirtyDays(ctx); err != nil || n < 1 {
		t.Fatalf("count the marked days: %d %v", n, err)
	}
	if v := data(api, "Alle", `{}`)["value"]; v != 6.0 {
		t.Fatalf("after the run of the minute: %v", v)
	}
	if v := data(api, "Summe", year)["value"]; v != 1.0+2+3+4+2+5 {
		t.Fatalf("sum after the run: %v", v)
	}

	// The person who reads her own records counts as before PL69: her one probe.
	if v := data(viewer.api, "Alle", `{}`)["value"]; v != 1.0 {
		t.Fatalf("viewer: %v", v)
	}
	if pts := data(viewer.api, "Nach Art", `{}`)["points"].([]any); len(pts) != 1 || pts[0].(map[string]any)["key"] != "wish" {
		t.Fatalf("viewer's groups: %v", pts)
	}
}
