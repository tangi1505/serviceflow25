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
	"time"

	"group24.de/itsm/internal/db/dbtest"
)

// The Prüfweg of UI13a (UI-Plan E39): a dashboard with widgets saved at once, refused as a
// whole when one widget is wrong; the numbers with the filters period and unit; another
// person, with whom it is shared, sees her own numbers and may not change it; the dashboard
// as the start page.
func TestDashboardsOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_ui13"))
	api := apiServer(t)
	if r := call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "ui13_nord"}`, nil); r.code != 201 {
		t.Fatalf("node: %d %s", r.code, r.raw)
	}
	create := func(a *testAPI, typeName, body string) map[string]any {
		t.Helper()
		r := call(t, a, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body
	}
	create(api, "probe.probe", `{"title": "UI13 eins", "kind": "fault"}`)
	create(api, "probe.probe", `{"title": "UI13 zwei", "kind": "fault", "org_path": "root.ui13_nord"}`)
	create(api, "probe.probe", `{"title": "UI13 drei", "kind": "question"}`)

	// The other person: reads her own records and the dashboards shared with her.
	const role = "ui13_viewer"
	dropRole(t, api, role)
	call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "Betrachter"}}`, nil)
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "own"}`, `{"type_name": "probe.probe", "action": "create", "scope": "all"}`,
		`{"type_name": "core.dashboard", "action": "read", "scope": "involved"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	viewer := timelineLogin(t, api, "apitest_ui13_viewer", role, `{"last_name": "Betrachterin"}`)

	d := create(api, "core.dashboard", `{"name": "UI13 Überblick", "actors": {"shared": [{"type": "core.person", "id": "`+viewer.person+`"}]}}`)
	id := d["id"].(string)
	other := create(api, "core.dashboard", `{"name": "UI13 privat"}`)
	path := "/api/v1/dashboards/" + id
	save := func(version any, widgets string) resp {
		return call(t, api, http.MethodPut, path, fmt.Sprintf(`{"version": %v, "fields": {"description": "für den Test"}, "widgets": [%s]}`, version, widgets), nil)
	}
	const (
		number = `{"title": "Alle", "kind": "number", "target_type": "probe.probe", "x": 0, "y": 0}`
		pie    = `{"title": "Nach Art", "kind": "pie", "target_type": "probe.probe", "group_by": "kind", "x": 1, "y": 0, "w": 2}`
		line   = `{"title": "Je Tag", "kind": "line", "target_type": "probe.probe", "group_by": "created_at:day", "x": 0, "y": 1, "w": 2}`
		list   = `{"title": "Neueste", "kind": "list", "target_type": "probe.probe", "columns": "title,kind", "sort": "-created_at", "size": 2, "x": 2, "y": 1, "w": 2}`
		text   = `{"title": "Hinweise", "kind": "text", "body": "<p>Wartung am Samstag</p>", "x": 0, "y": 2, "w": 4}`
		nounit = `{"title": "Alle Einheiten", "kind": "number", "target_type": "probe.probe", "use_unit": false, "x": 3, "y": 0}`
	)

	// One wrong widget refuses the whole save, and nothing is written.
	for _, c := range []struct{ widget, field string }{
		{`{"title": "Ohne Gruppe", "kind": "pie", "target_type": "probe.probe"}`, "widgets[1].group_by"},
		{`{"title": "Beides", "kind": "number", "target_type": "probe.probe", "saved_search": "` + id + `"}`, "widgets[1].saved_search"},
		{`{"title": "Summe", "kind": "number", "target_type": "probe.probe", "measure": "sum", "measure_field": "title"}`, "widgets[1].measure_field"},
		{`{"title": "Stunde", "kind": "line", "target_type": "probe.probe", "group_by": "created_at:hour"}`, "widgets[1].group_by"},
		{`{"title": "Falsch", "kind": "number", "target_type": "probe.probe", "query": "{\"field\": \"nope\", \"eq\": 1}"}`, "widgets[1].query"},
		{`{"title": "Ohne Quelle", "kind": "bar", "group_by": "kind"}`, "widgets[1].target_type"},
	} {
		r := save(d["version"], number+","+c.widget)
		if r.code != 422 || !strings.Contains(r.raw, c.field) {
			t.Fatalf("%s: %d %s", c.field, r.code, r.raw)
		}
	}
	if r := call(t, api, http.MethodGet, path, "", nil); r.code != 200 || len(r.body["widgets"].([]any)) != 0 || r.body["editable"] != true {
		t.Fatalf("nothing written: %d %s", r.code, r.raw)
	}
	r := save(d["version"], strings.Join([]string{text, list, line, pie, number, nounit}, ","))
	if r.code != 200 {
		t.Fatalf("save: %d %s", r.code, r.raw)
	}
	widgets := r.body["widgets"].([]any)
	titles := []string{}
	byTitle := map[string]string{}
	for _, w := range widgets {
		m := w.(map[string]any)
		titles = append(titles, m["title"].(string))
		byTitle[m["title"].(string)] = m["id"].(string)
	}
	if strings.Join(titles, "|") != "Alle|Nach Art|Alle Einheiten|Je Tag|Neueste|Hinweise" {
		t.Fatalf("order of the grid: %v", titles)
	}
	// The version of the dashboard guards the save.
	if r := save(d["version"], number); r.code != 409 {
		t.Fatalf("stale version: %d %s", r.code, r.raw)
	}

	data := func(a *testAPI, title, filters string) resp {
		t.Helper()
		r := call(t, a, http.MethodPost, path+"/widgets/"+byTitle[title]+"/data", filters, nil)
		if r.code != 200 {
			t.Fatalf("data of %s: %d %s", title, r.code, r.raw)
		}
		return r
	}
	if v := data(api, "Alle", `{}`).body["value"]; v != 3.0 {
		t.Fatalf("number: %v", v)
	}
	pts := data(api, "Nach Art", `{}`).body["points"].([]any)
	if len(pts) != 2 || pts[0].(map[string]any)["key"] != "fault" || pts[0].(map[string]any)["value"] != 2.0 ||
		pts[0].(map[string]any)["label"].(map[string]any)["de"] != "Störung" {
		t.Fatalf("pie: %v", pts)
	}
	today := time.Now().In(time.FixedZone("CET", 3600)).Format("2006-01-02")
	if pts := data(api, "Je Tag", `{}`).body["points"].([]any); len(pts) != 1 || pts[0].(map[string]any)["value"] != 3.0 ||
		(pts[0].(map[string]any)["key"] != today && pts[0].(map[string]any)["key"] != time.Now().UTC().Format("2006-01-02")) {
		t.Fatalf("line: %v", pts)
	}
	l := data(api, "Neueste", `{}`)
	if items := l.body["items"].([]any); len(items) != 2 || items[0].(map[string]any)["title"] != "UI13 drei" || len(l.body["columns"].([]any)) != 2 {
		t.Fatalf("list: %s", l.raw)
	}
	if r := data(api, "Hinweise", `{}`); r.body["kind"] != "text" || r.body["value"] != nil {
		t.Fatalf("text: %s", r.raw)
	}
	// The filters: the unit Nord has one, a period from tomorrow none; a widget without the
	// unit counts all.
	if v := data(api, "Alle", `{"unit": "root.ui13_nord"}`).body["value"]; v != 1.0 {
		t.Fatalf("unit: %v", v)
	}
	if v := data(api, "Alle Einheiten", `{"unit": "root.ui13_nord"}`).body["value"]; v != 3.0 {
		t.Fatalf("widget without the unit: %v", v)
	}
	tomorrow := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if v := data(api, "Alle", `{"from": "`+tomorrow+`"}`).body["value"]; v != 0.0 {
		t.Fatalf("period: %v", v)
	}
	if r := data(api, "Alle", `{"unit": "root.ui13_nord"}`); !strings.Contains(r.raw, `"under":"root.ui13_nord"`) {
		t.Fatalf("the condition behind the number: %s", r.raw)
	}
	if r := call(t, api, http.MethodPost, path+"/widgets/"+byTitle["Alle"]+"/data", `{"unit": "Nord"}`, nil); r.code != 422 {
		t.Fatalf("a unit that is no path: %d %s", r.code, r.raw)
	}

	// The other person sees her own numbers, may not change it, and does not see the other.
	create(viewer.api, "probe.probe", `{"title": "UI13 ihrer", "kind": "wish"}`)
	if r := call(t, viewer.api, http.MethodGet, path, "", nil); r.code != 200 || r.body["editable"] != false {
		t.Fatalf("viewer reads: %d %s", r.code, r.raw)
	}
	if v := data(viewer.api, "Alle", `{}`).body["value"]; v != 1.0 {
		t.Fatalf("viewer's number: %v", v)
	}
	if v := data(api, "Alle", `{}`).body["value"]; v != 4.0 {
		t.Fatalf("admin's number: %v", v)
	}
	cur := call(t, api, http.MethodGet, path, "", nil).body["dashboard"].(map[string]any)
	if r := call(t, viewer.api, http.MethodPut, path, fmt.Sprintf(`{"version": %v, "widgets": []}`, cur["version"]), nil); r.code != 403 && r.code != 404 {
		t.Fatalf("viewer saves: %d %s", r.code, r.raw)
	}
	if r := call(t, viewer.api, http.MethodGet, "/api/v1/dashboards/"+other["id"].(string), "", nil); r.code != 404 {
		t.Fatalf("viewer reads the private one: %d %s", r.code, r.raw)
	}
	if r := call(t, viewer.api, http.MethodPost, "/api/v1/dashboards/"+other["id"].(string)+"/widgets/"+byTitle["Alle"]+"/data", `{}`, nil); r.code != 404 {
		t.Fatalf("a widget through another dashboard: %d %s", r.code, r.raw)
	}

	// Saving again changes, keeps and removes.
	cur = call(t, api, http.MethodGet, path, "", nil).body["dashboard"].(map[string]any)
	r = save(cur["version"], `{"id": "`+byTitle["Alle"]+`", "title": "Alle Proben", "kind": "number", "target_type": "probe.probe", "x": 0, "y": 0}, `+
		`{"id": "`+byTitle["Hinweise"]+`", "title": "Hinweise", "kind": "text", "body": "<p>neu</p>", "x": 0, "y": 1, "w": 4}`)
	if r.code != 200 || len(r.body["widgets"].([]any)) != 2 || r.body["widgets"].([]any)[0].(map[string]any)["title"] != "Alle Proben" {
		t.Fatalf("save again: %d %s", r.code, r.raw)
	}
	var left int
	if err := api.pool.QueryRow(t.Context(), "SELECT count(*) FROM rec_core_dashboard_widget WHERE dashboard = $1", id).Scan(&left); err != nil || left != 2 {
		t.Fatalf("removed widgets are gone: %d %v", left, err)
	}

	// The start page: one's own choice, checked.
	if r := call(t, viewer.api, http.MethodPatch, "/api/v1/me/settings", `{"start_dashboard": "`+id+`"}`, nil); r.code != 200 {
		t.Fatalf("start page: %d %s", r.code, r.raw)
	}
	if r := call(t, viewer.api, http.MethodGet, "/api/v1/me/settings", "", nil); r.body["start_dashboard"].(map[string]any)["value"] != id {
		t.Fatalf("start page read: %s", r.raw)
	}
	if r := call(t, viewer.api, http.MethodPatch, "/api/v1/me/settings", `{"start_dashboard": "`+other["id"].(string)+`"}`, nil); r.code != 422 {
		t.Fatalf("a dashboard she may not read: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodPut, "/api/v1/org/nodes/root/settings/ui.start_dashboard", `"`+id+`"`, nil); r.code != 204 {
		t.Fatalf("start page at the root: %d %s", r.code, r.raw)
	}
}

// The Prüfweg of UI13b (UI-Plan E39): a widget in the sidebar of a person counts her probes
// with @record, one of a probe the probes of the same kind with @record.kind; it shows only
// for those who read its dashboard. A tile of the portal shows a number with the rights of the
// person who sees it.
func TestDashboardsEmbeddedOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_ui13b"))
	api := apiServer(t)
	create := func(a *testAPI, typeName, body string) map[string]any {
		t.Helper()
		r := call(t, a, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body
	}
	const role = "ui13b_viewer"
	dropRole(t, api, role)
	call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "Betrachter"}}`, nil)
	for _, p := range []string{`{"type_name": "probe.probe", "action": "read", "scope": "own"}`, `{"type_name": "probe.probe", "action": "create", "scope": "all"}`,
		`{"type_name": "core.person", "action": "read", "scope": "all"}`, `{"type_name": "core.dashboard", "action": "read", "scope": "involved"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	viewer := timelineLogin(t, api, "apitest_ui13b_viewer", role, `{"last_name": "Betrachterin"}`)
	nina := create(api, "core.person", `{"last_name": "Nordmann"}`)["id"].(string)
	asRequester := `"actors": {"requester": [{"type": "core.person", "id": "` + nina + `"}]}`
	create(api, "probe.probe", `{"title": "UI13b eins", "kind": "fault", `+asRequester+`}`)
	create(api, "probe.probe", `{"title": "UI13b zwei", "kind": "fault", `+asRequester+`}`)
	other := create(api, "probe.probe", `{"title": "UI13b drei", "kind": "question"}`)

	d := create(api, "core.dashboard", `{"name": "UI13b Seitenleiste"}`)
	id := d["id"].(string)
	save := func(widgets string) resp {
		cur := call(t, api, http.MethodGet, "/api/v1/dashboards/"+id, "", nil).body["dashboard"].(map[string]any)
		return call(t, api, http.MethodPut, "/api/v1/dashboards/"+id, fmt.Sprintf(`{"version": %v, "widgets": [%s]}`, cur["version"], widgets), nil)
	}
	const (
		byPerson = `{"title": "Ihre Proben", "kind": "number", "target_type": "probe.probe", "sidebar_type": "core.person", "query": "{\"field\": \"actors.requester\", \"eq\": \"@record\"}"}`
		sameKind = `{"title": "Gleiche Art", "kind": "number", "target_type": "probe.probe", "sidebar_type": "probe.probe", "query": "{\"field\": \"kind\", \"eq\": \"@record.kind\"}", "x": 1}`
		all      = `{"title": "Alle", "kind": "number", "target_type": "probe.probe", "x": 2}`
		pie      = `{"title": "Torte", "kind": "pie", "target_type": "probe.probe", "group_by": "kind", "x": 3}`
	)
	for _, c := range []struct{ widget, want string }{
		{`{"title": "Ohne Leiste", "kind": "number", "target_type": "probe.probe", "query": "{\"field\": \"parent\", \"eq\": \"@record\"}"}`, "set sidebar_type"},
		{`{"title": "Kein Feld", "kind": "number", "target_type": "probe.probe", "sidebar_type": "probe.probe", "query": "{\"field\": \"kind\", \"eq\": \"@record.nope\"}"}`, "no field"},
		{`{"title": "Kein Typ", "kind": "number", "target_type": "probe.probe", "sidebar_type": "nope.nope"}`, "not installed"},
	} {
		if r := save(c.widget); r.code != 422 || !strings.Contains(r.raw, c.want) {
			t.Fatalf("%s: %d %s", c.want, r.code, r.raw)
		}
	}
	r := save(strings.Join([]string{byPerson, sameKind, all, pie}, ","))
	if r.code != 200 {
		t.Fatalf("save: %d %s", r.code, r.raw)
	}
	byTitle := map[string]string{}
	for _, w := range r.body["widgets"].([]any) {
		m := w.(map[string]any)
		byTitle[m["title"].(string)] = m["id"].(string)
	}

	// The sidebar of Nina: her two probes; of the third probe: the one of its kind.
	if r := call(t, api, http.MethodGet, "/api/v1/records/core.person/"+nina+"/widgets", "", nil); r.code != 200 || !strings.Contains(r.raw, "Ihre Proben") || strings.Contains(r.raw, "Gleiche Art") {
		t.Fatalf("sidebar of a person: %d %s", r.code, r.raw)
	}
	data := func(a *testAPI, typeName, rec, title string) resp {
		return call(t, a, http.MethodPost, "/api/v1/records/"+typeName+"/"+rec+"/widgets/"+byTitle[title]+"/data", "", nil)
	}
	if r := data(api, "core.person", nina, "Ihre Proben"); r.code != 200 || r.body["value"] != 2.0 {
		t.Fatalf("her probes: %d %s", r.code, r.raw)
	}
	if r := data(api, "probe.probe", other["id"].(string), "Gleiche Art"); r.code != 200 || r.body["value"] != 1.0 {
		t.Fatalf("the same kind: %d %s", r.code, r.raw)
	}
	if r := data(api, "probe.probe", other["id"].(string), "Ihre Proben"); r.code != 404 {
		t.Fatalf("a widget of another type's sidebar: %d %s", r.code, r.raw)
	}
	// On its dashboard a widget with @record has no record to count for.
	if r := call(t, api, http.MethodPost, "/api/v1/dashboards/"+id+"/widgets/"+byTitle["Ihre Proben"]+"/data", `{}`, nil); r.code != 200 || r.body["sidebar_only"] != true {
		t.Fatalf("on the dashboard: %d %s", r.code, r.raw)
	}

	// Only for those who read the dashboard (your decision 2026-11-13).
	if r := call(t, viewer.api, http.MethodGet, "/api/v1/records/core.person/"+nina+"/widgets", "", nil); r.code != 200 || strings.TrimSpace(r.raw) != "[]" {
		t.Fatalf("not shared: %d %s", r.code, r.raw)
	}
	if r := data(viewer.api, "core.person", nina, "Ihre Proben"); r.code != 404 {
		t.Fatalf("not shared, data: %d %s", r.code, r.raw)
	}
	cur := call(t, api, http.MethodGet, "/api/v1/records/core.dashboard/"+id, "", nil).body
	if r := call(t, api, http.MethodPatch, "/api/v1/records/core.dashboard/"+id, `{"actors": {"shared": [{"type": "core.person", "id": "`+viewer.person+`"}]}}`,
		map[string]string{"If-Match": fmt.Sprintf(`"%v"`, cur["version"])}); r.code != 200 {
		t.Fatalf("share: %d %s", r.code, r.raw)
	}
	if r := call(t, viewer.api, http.MethodGet, "/api/v1/records/core.person/"+nina+"/widgets", "", nil); r.code != 200 || !strings.Contains(r.raw, "Ihre Proben") {
		t.Fatalf("shared: %d %s", r.code, r.raw)
	}
	// With her rights: she reads none of Nina's probes.
	if r := data(viewer.api, "core.person", nina, "Ihre Proben"); r.code != 200 || r.body["value"] != 0.0 {
		t.Fatalf("her rights: %d %s", r.code, r.raw)
	}

	// A tile of the portal: a number widget, not one of the sidebar, not a pie.
	tile := func(widget string) resp {
		return call(t, api, http.MethodPost, "/api/v1/records/core.portal_tile", `{"name": "Meine Proben", "kind": "dashboard", "widget": "`+widget+`"}`, nil)
	}
	if r := tile(byTitle["Ihre Proben"]); r.code != 422 || !strings.Contains(r.raw, "@record") {
		t.Fatalf("tile with @record: %d %s", r.code, r.raw)
	}
	if r := tile(byTitle["Torte"]); r.code != 422 || !strings.Contains(r.raw, "kind number") {
		t.Fatalf("tile with a pie: %d %s", r.code, r.raw)
	}
	r = tile(byTitle["Alle"])
	if r.code != 201 {
		t.Fatalf("tile: %d %s", r.code, r.raw)
	}
	tid := r.body["id"].(string)
	create(viewer.api, "probe.probe", `{"title": "UI13b ihre", "kind": "wish"}`)
	if r := call(t, viewer.api, http.MethodPost, "/api/v1/portal/tiles/"+tid+"/data", "", nil); r.code != 200 || r.body["value"] != 1.0 {
		t.Fatalf("tile for her: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/portal/tiles/"+tid+"/data", "", nil); r.code != 200 || r.body["value"] != 4.0 {
		t.Fatalf("tile for the admin: %d %s", r.code, r.raw)
	}
}
