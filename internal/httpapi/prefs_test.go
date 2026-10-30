// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strings"
	"testing"
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
	r = call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"language": "en", "time_zone": "America/New_York", "theme": "probe",
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
	if v, src := value(r, "theme"); v != "probe" || src != "own" {
		t.Fatalf("theme stays: %s", r.raw)
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
