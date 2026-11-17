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
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of UI14b (UI-Plan E40; Inventar G76, G77): after installing the service desk and
// the incidents, "Helpdesk" and "Statistik" are there, shared with the group Service Desk,
// with the widgets of the incidents; Helpdesk shows the open incidents by state. Installing
// the requests adds theirs. Every widget computes, alone and all in one request (PL67).
func TestDashboardsShippedOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_ui14b"))
	api := apiServer(t)
	ctx := t.Context()
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	install := func(dir string) {
		t.Helper()
		p := installPackage(t, ctx, api.rt, dir)
		if _, err := starter.Package(ctx, p); err != nil {
			t.Fatalf("start data of %s: %v", p.Name, err)
		}
	}
	install("../../packages/sd")
	install("../../packages/incident")
	var desk string
	if err := api.pool.QueryRow(ctx, "SELECT ref FROM package_seed WHERE org_path = 'root' AND package = 'sd' AND key = 'service_desk'").Scan(&desk); err != nil {
		t.Fatal(err)
	}
	board := func(key string) (string, map[string]any) {
		t.Helper()
		var id string
		if err := api.pool.QueryRow(ctx, "SELECT ref FROM package_seed WHERE org_path = 'root' AND package = 'sd' AND key = $1", key).Scan(&id); err != nil {
			t.Fatal(err)
		}
		r := call(t, api, http.MethodGet, "/api/v1/dashboards/"+id, "", nil)
		if r.code != 200 {
			t.Fatalf("%s: %d %s", key, r.code, r.raw)
		}
		return id, r.body
	}
	widgets := func(v map[string]any) map[string]string {
		out := map[string]string{}
		for _, w := range v["widgets"].([]any) {
			m := w.(map[string]any)
			out[m["title"].(string)] = m["id"].(string)
		}
		return out
	}
	hd, v := board("helpdesk")
	shared := v["dashboard"].(map[string]any)["actors"].(map[string]any)["shared"].([]any)
	if v["dashboard"].(map[string]any)["name"] != "Helpdesk" || len(shared) != 1 || shared[0].(map[string]any)["id"] != desk {
		t.Fatalf("helpdesk: %v", v["dashboard"])
	}
	if n := len(widgets(v)); n != 8 {
		t.Fatalf("helpdesk with the incidents: %d widgets", n)
	}

	// Open incidents by state: two new, one in progress, one resolved left out.
	create := func(body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/incident.incident", body, nil)
		if r.code != 201 {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	for _, title := range []string{"UI14b eins", "UI14b zwei", "UI14b drei"} {
		create(`{"title": "` + title + `", "impact": "low", "urgency": "low"}`)
	}
	started := create(`{"title": "UI14b vier", "impact": "low", "urgency": "low"}`)
	move := func(id, body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, "/api/v1/records/incident.incident/"+id, "", nil).body["version"]
		if r := call(t, api, http.MethodPost, "/api/v1/records/incident.incident/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("transition: %d %s", r.code, r.raw)
		}
	}
	move(started, `{"transition": "start"}`)
	data := func(dash, widget string) resp {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/dashboards/"+dash+"/widgets/"+widget+"/data", `{}`, nil)
		if r.code != 200 {
			t.Fatalf("widget data: %d %s", r.code, r.raw)
		}
		return r
	}
	w := widgets(v)
	byState := map[string]float64{}
	for _, p := range data(hd, w["Offene Störungen nach Status"]).body["points"].([]any) {
		m := p.(map[string]any)
		byState[m["key"].(string)] = m["value"].(float64)
	}
	if byState["new"] != 3 || byState["in_progress"] != 1 || len(byState) != 2 {
		t.Fatalf("open by state: %v", byState)
	}
	if r := data(hd, w["Offene Störungen"]); r.body["value"] != 4.0 {
		t.Fatalf("open: %s", r.raw)
	}

	// The requests add theirs; every widget of both computes.
	install("../../packages/request")
	_, v = board("helpdesk")
	if n := len(widgets(v)); n != 11 {
		t.Fatalf("helpdesk with the requests: %d widgets", n)
	}
	st, sv := board("statistik")
	if n := len(widgets(sv)); n != 16 {
		t.Fatalf("statistik: %d widgets", n)
	}
	for dash, view := range map[string]map[string]any{hd: v, st: sv} {
		for title, id := range widgets(view) {
			if r := data(dash, id); r.body["kind"] == nil {
				t.Fatalf("%s: %s", title, r.raw)
			}
		}
	}
	if r := data(st, widgets(sv)["Neue Störungen"]); r.body["value"] != 4.0 {
		t.Fatalf("new incidents: %s", r.raw)
	}

	// All widgets in one request (PL67): one line per widget, each the same as the widget
	// alone, also where several count in one pass; the same with a period.
	for _, filters := range []string{`{}`, `{"from": "2020-01-01T00:00:00Z"}`} {
		for dash, view := range map[string]map[string]any{hd: v, st: sv} {
			// call reads the first JSON value only; the stream is read to its end here.
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, api.URL+"/api/v1/dashboards/"+dash+"/data", strings.NewReader(filters))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfHeader, csrfValue)
			req.Header.Set("Cookie", sessionCookie+"="+api.cookie)
			res, err := api.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/x-ndjson" {
				t.Fatalf("dashboard data: %d %s %s", res.StatusCode, res.Header.Get("Content-Type"), raw)
			}
			lines := map[string]string{}
			for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				var line struct {
					Widget  string          `json:"widget"`
					Data    json.RawMessage `json:"data"`
					Problem json.RawMessage `json:"problem"`
				}
				if err := json.Unmarshal([]byte(l), &line); err != nil || len(line.Problem) > 0 {
					t.Fatalf("line %s: %v", l, err)
				}
				lines[line.Widget] = string(line.Data)
			}
			if len(lines) != len(widgets(view)) {
				t.Fatalf("%d lines for %d widgets: %s", len(lines), len(widgets(view)), raw)
			}
			for title, id := range widgets(view) {
				alone := call(t, api, http.MethodPost, "/api/v1/dashboards/"+dash+"/widgets/"+id+"/data", filters, nil)
				// The moment of the computation differs (PL68), the numbers do not.
				var a, b map[string]any
				_ = json.Unmarshal([]byte(alone.raw), &a)
				_ = json.Unmarshal([]byte(lines[id]), &b)
				delete(a, "computed_at")
				delete(b, "computed_at")
				if !reflect.DeepEqual(a, b) {
					t.Fatalf("%s %s: alone %s, in one request %s", title, filters, alone.raw, lines[id])
				}
			}
		}
	}
}
