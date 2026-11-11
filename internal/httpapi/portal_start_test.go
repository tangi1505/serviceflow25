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
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD14 (SD-Plan E13): the packages bring their tiles; a tile for a group shows
// only to its members, an inactive one to nobody; a notice written by hand stands under
// "Aktuell" from its start to its end, for whom it is meant, after the notices of a query; once
// it is over, the rule of the start data sets it expired.
func TestPortalStartOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd14"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/core", 1}, {"../../packages/sd", 28}, {"../../packages/incident", 40}, {"../../packages/knowledge", 9}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd14_user", "label": {"de": "SD14"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	north := timelineLogin(t, admin, "apitest_sd14_north", "sd14_user", `{"last_name": "Nord"}`)
	south := timelineLogin(t, admin, "apitest_sd14_south", "sd14_user", `{"last_name": "Süd"}`)
	group := call(t, admin, http.MethodPost, "/api/v1/records/core.group", `{"name": "SD14 Dienststelle Nord"}`, nil)
	if group.code != 201 {
		t.Fatalf("group: %d %s", group.code, group.raw)
	}
	gid := group.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+gid+`", "person": "`+north.person+`"}`, nil); r.code != 201 {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}

	// Tiles: the two of the packages for all, one for the group, an inactive one for nobody.
	const tiles = "/api/v1/records/core.portal_tile"
	for _, body := range []string{
		`{"name": "Zugang beantragen", "kind": "link", "url": "https://intranet.example/zugang", "position": 20, "audience": "groups",
			"actors": {"shared": [{"type": "core.group", "id": "` + gid + `"}]}}`,
		`{"name": "Alt", "kind": "link", "url": "https://intranet.example/alt", "active": false}`,
	} {
		if r := call(t, admin, http.MethodPost, tiles, body, nil); r.code != 201 {
			t.Fatalf("tile: %d %s", r.code, r.raw)
		}
	}
	if r := call(t, admin, http.MethodPost, tiles, `{"name": "Ohne Ziel", "kind": "form"}`, nil); r.code != 422 {
		t.Fatalf("a tile for a form without the form: %d %s", r.code, r.raw)
	}
	names := func(api *testAPI) []string {
		t.Helper()
		r := call(t, api, http.MethodPost, tiles+":query", `{"sort": ["position"], "limit": 50}`, nil)
		if r.code != 200 {
			t.Fatalf("tiles: %d %s", r.code, r.raw)
		}
		var out []string
		for _, it := range r.body["items"].([]any) {
			out = append(out, it.(map[string]any)["name"].(string))
		}
		return out
	}
	if got := names(north.api); !slices.Equal(got, []string{"Störung melden", "Zugang beantragen", "Wissen"}) {
		t.Fatalf("the member of the group sees %v", got)
	}
	if got := names(south.api); !slices.Equal(got, []string{"Störung melden", "Wissen"}) {
		t.Fatalf("the other sees %v", got)
	}

	// Notices written by hand: within their time, for whom they are meant.
	const notes = "/api/v1/records/core.announcement"
	now := time.Now().UTC()
	stamp := func(d time.Duration) string { return `"` + now.Add(d).Format(time.RFC3339) + `"` }
	announce := func(title, from, until, extra string, publish bool) string {
		t.Helper()
		body := `{"title": "` + title + `", "body": "<p>` + title + ` – Text</p>"`
		if from != "" {
			body += `, "valid_from": ` + from
		}
		if until != "" {
			body += `, "valid_until": ` + until
		}
		r := call(t, admin, http.MethodPost, notes, body+extra+`}`, nil)
		if r.code != 201 {
			t.Fatalf("announcement %s: %d %s", title, r.code, r.raw)
		}
		id := r.body["id"].(string)
		if publish {
			if r := call(t, admin, http.MethodPost, notes+"/"+id+":transition", `{"transition": "publish"}`, nil); r.code != 200 {
				t.Fatalf("publish %s: %d %s", title, r.code, r.raw)
			}
		}
		return id
	}
	announce("Wartung VPN", stamp(-time.Hour), stamp(2*time.Hour), "", true)
	announce("Später", stamp(time.Hour), "", "", true)
	over := announce("Vorbei", stamp(-3*time.Hour), stamp(-time.Minute), "", true)
	announce("Entwurf", "", "", "", false)
	announce("Nur Nord", "", "", `, "audience": "groups", "actors": {"shared": [{"type": "core.group", "id": "`+gid+`"}]}`, true)
	type item struct {
		Notice *string `json:"notice"`
		Title  string  `json:"title"`
		Text   string  `json:"text"`
		Format string  `json:"format"`
		Until  *string `json:"until"`
	}
	current := func(api *testAPI) []item {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/portal/notices", "", nil)
		var out []item
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &out) != nil {
			t.Fatalf("notices: %d %s", r.code, r.raw)
		}
		return out
	}
	titles := func(list []item) []string {
		var out []string
		for _, it := range list {
			out = append(out, it.Title)
		}
		slices.Sort(out)
		return out
	}
	south1 := current(south.api)
	if got := titles(south1); !slices.Equal(got, []string{"Wartung VPN"}) {
		t.Fatalf("the other reads %v", got)
	}
	if it := south1[0]; it.Notice != nil || it.Format != "html" || it.Text != "<p>Wartung VPN – Text</p>" || it.Until == nil {
		t.Fatalf("the notice written by hand: %+v", it)
	}
	if got := titles(current(north.api)); !slices.Equal(got, []string{"Nur Nord", "Wartung VPN"}) {
		t.Fatalf("the member of the group reads %v", got)
	}
	// The administrator reads more by his grant, but under "Aktuell" only what stands now.
	if got := titles(current(admin)); !slices.Equal(got, []string{"Nur Nord", "Wartung VPN"}) {
		t.Fatalf("the administrator reads under Aktuell %v", got)
	}

	// The rule of the start data sets the one that is over expired.
	if _, err := admin.rules.RunSchedules(ctx, now.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodGet, notes+"/"+over, "", nil); r.body["state"] != "expired" {
		t.Fatalf("the notice that is over: %v", r.body["state"])
	}
}
