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
	"strings"
	"testing"

	"group24.de/itsm/internal/prefs"
)

// One's own settings (UI9): each value with where it comes from; own before the tree before
// the default; the language at the person, without a right on core.person; null gives the
// own value back to the tree; wrong values are refused.
func TestSettingsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	timelineRoles(t, admin, "ui9")
	u := timelineLogin(t, admin, "apitest_ui9", "ui9_reader", `{"last_name": "UI9"}`)
	call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/settings/ui.time_zone", "", nil)
	call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/settings/ui.language", "", nil)
	defer call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/settings/ui.time_zone", "", nil)
	value := func(r resp, key string) (string, string) {
		m, _ := r.body[key].(map[string]any)
		return fmtValue(m["value"]), fmtValue(m["source"])
	}

	r := call(t, u.api, http.MethodGet, "/api/v1/me/settings", "", nil)
	if v, src := value(r, "time_zone"); r.code != 200 || v != "Europe/Berlin" || src != "default" {
		t.Fatalf("defaults: %d %s", r.code, r.raw)
	}
	if v, src := value(r, "language"); v != "de" || src != "own" {
		t.Fatalf("language of a new person: %s", r.raw)
	}
	// The tree sets a time zone for everyone below the root.
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/ui.time_zone", `"Europe/London"`, nil); r.code != 204 {
		t.Fatalf("global: %d %s", r.code, r.raw)
	}
	r = call(t, u.api, http.MethodGet, "/api/v1/me/settings", "", nil)
	if v, src := value(r, "time_zone"); v != "Europe/London" || src != "root" {
		t.Fatalf("global time zone: %s", r.raw)
	}

	// Own values win; the language goes to the person although the role may only read probe.
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"language": "en", "time_zone": "America/New_York",
		"defaults": {"probe.probe": {"kind": "fault", "priority": 2}}}`, nil)
	if v, src := value(r, "time_zone"); r.code != 200 || v != "America/New_York" || src != "own" {
		t.Fatalf("own: %d %s", r.code, r.raw)
	}
	if v, _ := value(r, "language"); v != "en" || !strings.Contains(r.raw, `"probe.probe":{"kind":"fault","priority":2}`) {
		t.Fatalf("own language and presets: %s", r.raw)
	}
	if p := call(t, admin, http.MethodGet, "/api/v1/records/core.person/"+u.person, "", nil); p.body["language"] != "en" {
		t.Fatalf("language at the person: %s", p.raw)
	}
	// null gives the value back: the time zone to the tree, the language to the default.
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"language": null, "time_zone": null, "defaults": {"probe.probe": null}}`, nil)
	if v, src := value(r, "time_zone"); v != "Europe/London" || src != "root" {
		t.Fatalf("time zone back to the tree: %s", r.raw)
	}
	if v, src := value(r, "language"); v != "de" || src != "default" || strings.Contains(r.raw, "probe.probe") {
		t.Fatalf("language back to the default: %s", r.raw)
	}
	// The theme is the organisation's (UI38, E49): a person sets none, null still goes.
	if v, src := value(r, "theme"); v != "neutral" || src != "default" {
		t.Fatalf("theme: %s", r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"theme": "probe"}`, nil); r.code != 422 || !strings.Contains(r.raw, "organisation") {
		t.Fatalf("own theme: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"theme": null}`, nil); r.code != 200 {
		t.Fatalf("theme null: %d %s", r.code, r.raw)
	}

	// The colour scheme (UI29): system by default, the tree's value, then one's own.
	if v, src := value(r, "color_scheme"); v != "system" || src != "default" {
		t.Fatalf("colour scheme default: %s", r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/ui.color_scheme", `"light"`, nil); r.code != 204 {
		t.Fatalf("global colour scheme: %d %s", r.code, r.raw)
	}
	defer call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/settings/ui.color_scheme", "", nil)
	r = call(t, u.api, http.MethodGet, "/api/v1/me/settings", "", nil)
	if v, src := value(r, "color_scheme"); v != "light" || src != "root" {
		t.Fatalf("colour scheme of the tree: %s", r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"color_scheme": "dark"}`, nil)
	if v, src := value(r, "color_scheme"); r.code != 200 || v != "dark" || src != "own" {
		t.Fatalf("own colour scheme: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"color_scheme": "pink"}`, nil); r.code != 422 || !strings.Contains(r.raw, "light, dark or system") {
		t.Fatalf("bad colour scheme: %d %s", r.code, r.raw)
	}
	// The row height of lists (UI30): normal by default, compact one's own.
	if v, src := value(r, "density"); v != "normal" || src != "default" {
		t.Fatalf("density default: %s", r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"density": "compact"}`, nil)
	if v, src := value(r, "density"); r.code != 200 || v != "compact" || src != "own" {
		t.Fatalf("own density: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"density": "tight"}`, nil); r.code != 422 || !strings.Contains(r.raw, "compact or normal") {
		t.Fatalf("bad density: %d %s", r.code, r.raw)
	}
	// The width of the navigation (UI36): wide by default, narrow one's own, null back.
	if v, src := value(r, "nav"); v != "wide" || src != "default" {
		t.Fatalf("nav default: %s", r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"nav": "narrow"}`, nil)
	if v, src := value(r, "nav"); r.code != 200 || v != "narrow" || src != "own" {
		t.Fatalf("own nav: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"nav": "half"}`, nil); r.code != 422 || !strings.Contains(r.raw, "wide or narrow") {
		t.Fatalf("bad nav: %d %s", r.code, r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"nav": null}`, nil)
	if v, _ := value(r, "nav"); v != "wide" {
		t.Fatalf("nav removed: %s", r.raw)
	}
	// Single keys as shortcuts (UI52): on by default, off one's own, others refused, null back.
	if v, src := value(r, "shortcuts"); v != "on" || src != "default" {
		t.Fatalf("shortcuts default: %s", r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"shortcuts": "off"}`, nil)
	if v, src := value(r, "shortcuts"); r.code != 200 || v != "off" || src != "own" {
		t.Fatalf("own shortcuts: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"shortcuts": "some"}`, nil); r.code != 422 || !strings.Contains(r.raw, "on or off") {
		t.Fatalf("bad shortcuts: %d %s", r.code, r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"shortcuts": null}`, nil)
	if v, _ := value(r, "shortcuts"); v != "on" {
		t.Fatalf("shortcuts removed: %s", r.raw)
	}
	// The accent colour (UI38): petrol by default, one of the six as one's own, others refused.
	if v, src := value(r, "accent"); v != "petrol" || src != "default" {
		t.Fatalf("accent default: %s", r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"accent": "indigo"}`, nil)
	if v, src := value(r, "accent"); r.code != 200 || v != "indigo" || src != "own" {
		t.Fatalf("own accent: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"accent": "red"}`, nil); r.code != 422 || !strings.Contains(r.raw, "petrol, steel") {
		t.Fatalf("red accent: %d %s", r.code, r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"accent": null}`, nil)
	if v, _ := value(r, "accent"); v != "petrol" {
		t.Fatalf("accent removed: %s", r.raw)
	}
	// The arrangement of the navigation (UI37, UI-Plan E50): empty lists without own ones; kept
	// in their order; repeats, odd keys and too many are refused; null and [] take them away.
	for _, name := range []string{"nav_order", "nav_hidden", "favorites"} {
		if l, ok := r.body[name].([]any); !ok || len(l) != 0 {
			t.Fatalf("%s empty by default: %s", name, r.raw)
		}
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"nav_order": ["package.probe", "dashboards"],
		"nav_hidden": ["saved"], "favorites": ["package.probe/probe.probe", "dashboards/7c9e6679-7425-40de-944b-e07fc1f90ae7"]}`, nil)
	if r.code != 200 || fmt.Sprint(r.body["nav_order"]) != "[package.probe dashboards]" || fmt.Sprint(r.body["nav_hidden"]) != "[saved]" ||
		fmt.Sprint(r.body["favorites"]) != "[package.probe/probe.probe dashboards/7c9e6679-7425-40de-944b-e07fc1f90ae7]" {
		t.Fatalf("own arrangement: %d %s", r.code, r.raw)
	}
	many := make([]string, prefs.MaxNavKeys+1)
	for i := range many {
		many[i] = fmt.Sprintf(`"k%d"`, i)
	}
	for body, want := range map[string]string{
		`{"favorites": ["a", "a"]}`:                         "distinct keys",
		`{"favorites": ["<script>"]}`:                       "distinct keys",
		`{"nav_order": "package.probe"}`:                    "list of keys",
		`{"nav_hidden": [` + strings.Join(many, ",") + `]}`: "at most 200",
	} {
		if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", body, nil); r.code != 422 || !strings.Contains(r.raw, want) {
			t.Fatalf("%s: %d %s", body, r.code, r.raw)
		}
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"nav_order": null, "nav_hidden": [], "favorites": null}`, nil)
	if r.code != 200 || fmt.Sprint(r.body["nav_order"], r.body["nav_hidden"], r.body["favorites"]) != "[] [] []" {
		t.Fatalf("arrangement removed: %d %s", r.code, r.raw)
	}
	// The last period per dashboard (PL65): none by default; kept per id; only the periods of
	// the list; null takes one away.
	const board = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	if m, ok := r.body["dashboard_periods"].(map[string]any); !ok || len(m) != 0 {
		t.Fatalf("no periods by default: %s", r.raw)
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"dashboard_periods": {"`+board+`": "days_30"}}`, nil)
	if m, _ := r.body["dashboard_periods"].(map[string]any); r.code != 200 || m[board] != "days_30" {
		t.Fatalf("period kept: %d %s", r.code, r.raw)
	}
	for body, want := range map[string]string{
		`{"dashboard_periods": {"` + board + `": "week"}}`: "a period is all",
		`{"dashboard_periods": {"statistik": "days_7"}}`:   "no id of a dashboard",
		`{"dashboard_periods": "days_7"}`:                  "object of dashboard ids",
	} {
		if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", body, nil); r.code != 422 || !strings.Contains(r.raw, want) {
			t.Fatalf("%s: %d %s", body, r.code, r.raw)
		}
	}
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"dashboard_periods": {"`+board+`": null}}`, nil)
	if m, _ := r.body["dashboard_periods"].(map[string]any); r.code != 200 || len(m) != 0 {
		t.Fatalf("period removed: %d %s", r.code, r.raw)
	}
	// The packages with label and symbol (UI36): every session reads them; the core is
	// „Stammdaten“ with the symbol database.
	pk := call(t, u.api, http.MethodGet, "/api/v1/packages", "", nil)
	if pk.code != 200 || !strings.Contains(pk.raw, `"name":"core"`) || !strings.Contains(pk.raw, `"Stammdaten"`) || !strings.Contains(pk.raw, `"icon":"database"`) {
		t.Fatalf("packages: %d %s", pk.code, pk.raw)
	}

	for body, field := range map[string]string{
		`{"time_zone": "Mars/Base"}`:                       "time_zone",
		`{"theme": "Großes Thema"}`:                        "theme",
		`{"language": "fr"}`:                               "language",
		`{"defaults": {"probe.probe": {"nofield": 1}}}`:    "defaults",
		`{"defaults": {"nothing.here": {}}}`:               "defaults",
		`{"page_size": 50}`:                                "page_size",
		`{"defaults": {"probe.probe": {"cost_total": 1}}}`: "defaults",
	} {
		if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", body, nil); r.code != 422 || !strings.Contains(r.raw, `"field":"`+field+`"`) {
			t.Fatalf("%s: %d %s", body, r.code, r.raw)
		}
	}
}

func fmtValue(v any) string {
	s, _ := v.(string)
	return s
}
