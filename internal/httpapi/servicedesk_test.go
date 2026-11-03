// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
)

// The Prüfweg of SD1 (SD-Plan E4): the package sd brings the lists with their provisional
// values in German and English and the trait ticket; a category in three levels; a category
// only for the Dienststelle Nord is seen by a person from Nord, not by one from Süd (N5).
func TestServiceDeskBaseOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd1"))
	admin := apiServer(t)
	installPackage(t, t.Context(), admin.rt, "../../packages/sd")
	const cats = "/api/v1/records/sd.category"
	names := func(api *testAPI, lang string) map[string]map[string]any {
		t.Helper()
		var hdr map[string]string
		if lang != "" {
			hdr = map[string]string{"Accept-Language": lang}
		}
		r := call(t, api, http.MethodGet, cats+"?limit=200", "", hdr)
		if r.code != 200 {
			t.Fatalf("categories: %d %s", r.code, r.raw)
		}
		out := map[string]map[string]any{}
		for _, it := range r.body["items"].([]any) {
			rec := it.(map[string]any)
			out[rec["name"].(string)] = rec
		}
		return out
	}

	// The shipped tree, in German and in English.
	de := names(admin, "")
	if len(de) != 17 || de["Drucker"] == nil || de["Drucker"]["full_name"] != "Hardware > Drucker" {
		t.Fatalf("shipped categories: %d %v", len(de), de["Drucker"])
	}
	if en := names(admin, "en"); en["Printer"] == nil || en["Specialist application"] == nil {
		t.Fatalf("English names: %v", en)
	}
	for _, list := range []string{"sd.channel", "sd.solution_kind"} {
		if r := call(t, admin, http.MethodGet, "/api/v1/records/"+list+"?limit=50", "", nil); r.code != 200 || len(r.body["items"].([]any)) < 5 {
			t.Fatalf("%s: %d %s", list, r.code, r.raw)
		}
	}
	// The trait with its fields and roles is installed for the incident and the request.
	if r := call(t, admin, http.MethodGet, "/api/v1/traits", "", nil); r.code != 200 ||
		!strings.Contains(r.raw, `"name":"ticket"`) || !strings.Contains(r.raw, `"assignee"`) || !strings.Contains(r.raw, `"sd.category"`) {
		t.Fatalf("trait ticket: %d %.600s", r.code, r.raw)
	}

	// A third level below Hardware > Drucker.
	laser := call(t, admin, http.MethodPost, cats, `{"name": "Laserdrucker", "parent": "`+de["Drucker"]["id"].(string)+`"}`, nil)
	if laser.code != 201 || laser.body["full_name"] != "Hardware > Drucker > Laserdrucker" {
		t.Fatalf("third level: %d %s", laser.code, laser.raw)
	}

	// A category for Nord only: a person of Nord sees it, one of Süd does not.
	for _, n := range []string{"nord", "sued"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "`+n+`"}`, nil); r.code != 201 {
			t.Fatalf("node %s: %d %s", n, r.code, r.raw)
		}
	}
	nordOnly := call(t, admin, http.MethodPost, cats, `{"name": "Nur Nord", "parent": "`+de["Software"]["id"].(string)+`", "org_path": "root.nord"}`, nil)
	if nordOnly.code != 201 {
		t.Fatalf("category of Nord: %d %s", nordOnly.code, nordOnly.raw)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd1_user", "label": {"de": "SD1"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	nord := timelineLogin(t, admin, "apitest_sd1_nord", "sd1_user", `{"last_name": "Nord", "org_path": "root.nord"}`)
	sued := timelineLogin(t, admin, "apitest_sd1_sued", "sd1_user", `{"last_name": "Süd", "org_path": "root.sued"}`)
	if got := names(nord.api, ""); got["Nur Nord"] == nil || got["Drucker"] == nil {
		t.Fatalf("Nord sees: %v", slices.Sorted(maps.Keys(got)))
	}
	got := names(sued.api, "")
	if got["Nur Nord"] != nil || got["Drucker"] == nil || len(got) != 18 {
		t.Fatalf("Süd sees: %v", slices.Sorted(maps.Keys(got)))
	}
	if r := call(t, sued.api, http.MethodGet, cats+"/"+nordOnly.body["id"].(string), "", nil); r.code != 404 {
		t.Fatalf("Süd reads the category of Nord: %d %s", r.code, r.raw)
	}
	// The administrator, with a grant of his own on the list, sees every entry.
	if all := names(admin, ""); all["Nur Nord"] == nil || len(all) != 19 {
		t.Fatalf("the administrator sees %d", len(all))
	}
}
