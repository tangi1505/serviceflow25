// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
)

// The Prüfweg of SD16a (SD-Plan E8, N12): persons without any grant on articles read the
// published ones for everyone at their node and above, those shared with a group of theirs
// wherever they lie, and neither drafts nor those only for agents; the search keeps to the
// same; nobody changes an article by reading it.
func TestKnowledgeOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd16a"))
	admin := apiServer(t)
	for _, dir := range []string{"../../packages/sd", "../../packages/knowledge"} {
		installPackage(t, t.Context(), admin.rt, dir)
	}
	const arts = "/api/v1/records/knowledge.article"

	// The type offers the area "Wissen" in the portal and brings its readers.
	ty := call(t, admin, http.MethodGet, "/api/v1/types/knowledge.article", "", nil)
	if ty.code != 200 || !strings.Contains(ty.raw, `"readers"`) || !strings.Contains(ty.raw, `"group":"category"`) {
		t.Fatalf("type: %d %.600s", ty.code, ty.raw)
	}

	for _, n := range []string{"nord", "sued"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "`+n+`"}`, nil); r.code != 201 {
			t.Fatalf("node %s: %d %s", n, r.code, r.raw)
		}
	}
	// A role without any permission: what these persons read, the readers give.
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd16_user", "label": {"de": "SD16"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	nord := timelineLogin(t, admin, "apitest_sd16_nord", "sd16_user", `{"last_name": "Nord", "org_path": "root.nord"}`)
	sued := timelineLogin(t, admin, "apitest_sd16_sued", "sd16_user", `{"last_name": "Süd", "org_path": "root.sued"}`)
	g := call(t, admin, http.MethodPost, "/api/v1/records/core.group", `{"name": "Netz SD16"}`, nil)
	if g.code != 201 {
		t.Fatalf("group: %d %s", g.code, g.raw)
	}
	group := g.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+group+`", "person": "`+sued.person+`"}`, nil); r.code != 201 {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}

	article := func(title, audience, node string, publish bool, extra string) string {
		t.Helper()
		body := `{"title": "` + title + `", "body": "<p>` + title + `: VPN neu starten.</p>", "audience": "` + audience + `"`
		if node != "" {
			body += `, "org_path": "` + node + `"`
		}
		r := call(t, admin, http.MethodPost, arts, body+extra+`}`, nil)
		if r.code != 201 || r.body["state"] != "draft" {
			t.Fatalf("article %s: %d %s", title, r.code, r.raw)
		}
		id := r.body["id"].(string)
		if publish {
			if r := call(t, admin, http.MethodPost, arts+"/"+id+":transition", `{"transition": "publish"}`, nil); r.code != 200 || r.body["state"] != "published" {
				t.Fatalf("publish %s: %d %s", title, r.code, r.raw)
			}
		}
		return id
	}
	forAll := article("Für alle", "all", "", true, "")
	article("Nur Nord", "all", "root.nord", true, "")
	article("Entwurf", "all", "", false, "")
	agents := article("Nur Bearbeiter", "agents", "", true, "")
	article("Gruppe Netz", "groups", "root.nord", true, `, "actors": {"shared": [{"type": "core.group", "id": "`+group+`"}]}`)

	titles := func(api *testAPI) []string {
		t.Helper()
		r := call(t, api, http.MethodGet, arts+"?limit=50", "", nil)
		if r.code != 200 {
			t.Fatalf("articles: %d %s", r.code, r.raw)
		}
		var out []string
		for _, it := range r.body["items"].([]any) {
			out = append(out, it.(map[string]any)["title"].(string))
		}
		slices.Sort(out)
		return out
	}
	if got := titles(nord.api); !slices.Equal(got, []string{"Für alle", "Nur Nord"}) {
		t.Fatalf("Nord reads %v", got)
	}
	if got := titles(sued.api); !slices.Equal(got, []string{"Für alle", "Gruppe Netz"}) {
		t.Fatalf("Süd, member of the group, reads %v", got)
	}
	if got := titles(admin); len(got) != 5 {
		t.Fatalf("the administrator reads %v", got)
	}
	// One by one the same; reading is not changing.
	if r := call(t, nord.api, http.MethodGet, arts+"/"+agents, "", nil); r.code != 404 {
		t.Fatalf("Nord reads the article for agents: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodGet, arts+"/"+forAll, "", nil); r.code != 200 || !strings.Contains(r.raw, "VPN neu starten") {
		t.Fatalf("Nord reads the article for all: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodPatch, arts+"/"+forAll, `{"title": "geändert"}`, map[string]string{"If-Match": `"1"`}); r.code != 403 && r.code != 404 {
		t.Fatalf("Nord changes the article: %d %s", r.code, r.raw)
	}

	// An English title (SD16c): read in English where there is one, else German.
	if r := call(t, admin, http.MethodPatch, arts+"/"+forAll, `{"title": {"en": "For everyone"}}`, map[string]string{"If-Match": `"2"`}); r.code != 200 {
		t.Fatalf("English title: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodGet, arts+"/"+forAll, "", map[string]string{"Accept-Language": "en"}); r.code != 200 || r.body["title"] != "For everyone" || !strings.Contains(r.raw, "VPN neu starten") {
		t.Fatalf("Nord reads in English: %d %s", r.code, r.raw)
	}
	if r := call(t, nord.api, http.MethodGet, arts+"/"+forAll, "", map[string]string{"Accept-Language": "de"}); r.body["title"] != "Für alle" {
		t.Fatalf("Nord reads in German: %d %s", r.code, r.raw)
	}

	// The search keeps to the readers: every article says VPN, Süd finds two.
	r := call(t, sued.api, http.MethodGet, "/api/v1/search?q=VPN&types=knowledge.article", "", nil)
	var hits []map[string]any
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &hits) != nil {
		t.Fatalf("search: %d %s", r.code, r.raw)
	}
	found := map[string]bool{}
	for _, h := range hits {
		found[h["title"].(string)] = true
	}
	if got := slices.Sorted(maps.Keys(found)); !slices.Equal(got, []string{"Für alle", "Gruppe Netz"}) {
		t.Fatalf("Süd finds %v", got)
	}
}
