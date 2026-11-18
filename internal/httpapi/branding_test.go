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
)

// The appearance of the organisation (UI39, UI-Plan E51): the sign-in page reads name, colour
// and logo of the root without a session; a colour below 4.5 : 1 on white is refused; the values
// hold for a whole tenant, not for a unit; a logo is a PNG or an SVG of at most 256 KB, served
// only as an image; what the organisation switches off, a person no longer sets herself.
func TestBrandingOverHTTP(t *testing.T) {
	admin := apiServer(t)
	anon := map[string]string{"Cookie": ""}
	keys := []string{"ui.app_name", "ui.accent", "ui.primary_accent", "ui.own_accent"}
	clean := func() {
		for _, k := range keys {
			call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/settings/"+k, "", nil)
		}
		call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/logo/light", "", nil)
		call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root/logo/dark", "", nil)
	}
	clean()
	defer clean()

	r := call(t, admin, http.MethodGet, "/api/v1/branding", "", anon)
	if r.code != 200 || r.body["name"] != "ServiceFlow25" || r.body["accent"] != "#0B6E80" || r.body["primary_accent"] != false ||
		r.body["logo_light"] != nil || r.body["logo_dark"] != nil {
		t.Fatalf("defaults without a session: %d %s", r.code, r.raw)
	}
	if me := call(t, admin, http.MethodGet, "/api/v1/auth/me", "", nil); me.body["appearance"] != true {
		t.Fatalf("the admin may change the appearance: %s", me.raw)
	}

	// 4.48 : 1 is too little, 4.54 : 1 enough; the name holds for the whole tenant.
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/ui.accent", `"#777777"`, nil); r.code != 422 || !strings.Contains(r.raw, "4.4 : 1 on white") {
		t.Fatalf("weak colour: %d %s", r.code, r.raw)
	}
	// The page saves all at once through the settings of the node.
	if r := call(t, admin, http.MethodPatch, "/api/v1/org/nodes/root/settings", `{"ui.app_name": "Leitstelle Nord", "ui.accent": "#767676", "ui.primary_accent": true}`, nil); r.code != 204 {
		t.Fatalf("save the appearance: %d %s", r.code, r.raw)
	}
	call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.ui39unit", "", nil)
	if unit := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "ui39unit"}`, nil); unit.code != 201 {
		t.Fatalf("unit: %d %s", unit.code, unit.raw)
	}
	defer call(t, admin, http.MethodDelete, "/api/v1/org/nodes/root.ui39unit", "", nil)
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root.ui39unit/settings/ui.app_name", `"Abteilung"`, nil); r.code != 422 || !strings.Contains(r.raw, "tenant") {
		t.Fatalf("name at a unit: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root.ui39unit/logo/light", "\x89PNG\r\n\x1a\nunit", nil); r.code != 422 {
		t.Fatalf("logo at a unit: %d %s", r.code, r.raw)
	}

	// A logo: PNG and SVG go, other content and too much do not.
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 100)
	r = call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/logo/light", png, nil)
	light, _ := r.body["logo_light"].(string)
	if r.code != 200 || !strings.HasPrefix(light, "/api/v1/me/branding/logo/light?v=") {
		t.Fatalf("png: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/logo/dark", `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, nil); r.code != 200 {
		t.Fatalf("svg: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/logo/dark", "<html>no image</html>", nil); r.code != 422 || !strings.Contains(r.raw, "PNG or an SVG") {
		t.Fatalf("html: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/logo/dark", "\x89PNG\r\n\x1a\n"+strings.Repeat("x", 300<<10), nil); r.code != 422 || !strings.Contains(r.raw, "256 KB") {
		t.Fatalf("too large: %d %s", r.code, r.raw)
	}

	// The sign-in page sees it all without a session.
	r = call(t, admin, http.MethodGet, "/api/v1/branding", "", anon)
	public, _ := r.body["logo_light"].(string)
	if r.code != 200 || r.body["name"] != "Leitstelle Nord" || r.body["accent"] != "#767676" || r.body["primary_accent"] != true ||
		!strings.HasPrefix(public, "/api/v1/branding/logo/light?v=") || r.body["logo_dark"] == nil {
		t.Fatalf("appearance without a session: %d %s", r.code, r.raw)
	}
	img := call(t, admin, http.MethodGet, public, "", anon)
	// raw holds only what a JSON decoder read; the length says the image came whole.
	if img.code != 200 || img.hdr.Get("Content-Length") != fmt.Sprint(len(png)) || img.hdr.Get("Content-Type") != "image/png" ||
		!strings.Contains(img.hdr.Get("Content-Security-Policy"), "sandbox") || !strings.Contains(img.hdr.Get("Cache-Control"), "immutable") {
		t.Fatalf("logo: %d %v", img.code, img.hdr)
	}
	svg := call(t, admin, http.MethodGet, "/api/v1/branding/logo/dark", "", anon)
	if svg.hdr.Get("Content-Type") != "image/svg+xml" || svg.hdr.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(svg.hdr.Get("Content-Security-Policy"), "sandbox") || svg.hdr.Get("Cache-Control") != "no-cache" {
		t.Fatalf("svg only as an image: %v", svg.hdr)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/me/branding", "", nil); r.body["name"] != "Leitstelle Nord" {
		t.Fatalf("appearance of the tenant: %s", r.raw)
	}
	// Without a session nothing else opens.
	if r := call(t, admin, http.MethodGet, "/api/v1/me/branding", "", anon); r.code != 401 {
		t.Fatalf("my appearance without a session: %d", r.code)
	}

	// What the organisation switches off, a person does not set herself; her value goes.
	timelineRoles(t, admin, "ui39")
	u := timelineLogin(t, admin, "apitest_ui39", "ui39_reader", `{"last_name": "UI39"}`)
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"accent": "indigo"}`, nil); r.code != 200 {
		t.Fatalf("own accent: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/root/settings/ui.own_accent", `false`, nil); r.code != 204 {
		t.Fatalf("switch off: %d %s", r.code, r.raw)
	}
	r = call(t, u.api, http.MethodGet, "/api/v1/me/settings", "", nil)
	allowed, _ := r.body["allowed"].(map[string]any)
	accent, _ := r.body["accent"].(map[string]any)
	if allowed["accent"] != false || allowed["density"] != true || accent["value"] != "#767676" || accent["source"] != "root" {
		t.Fatalf("accent of the organisation: %s", r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"accent": "steel"}`, nil); r.code != 422 || !strings.Contains(r.raw, "ui.own_accent") {
		t.Fatalf("own accent switched off: %d %s", r.code, r.raw)
	}
	if r := call(t, u.api, http.MethodPatch, "/api/v1/me/settings", `{"accent": null}`, nil); r.code != 200 {
		t.Fatalf("null still goes: %d %s", r.code, r.raw)
	}
}
