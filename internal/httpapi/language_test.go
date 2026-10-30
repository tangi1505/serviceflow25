// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestPickLanguage(t *testing.T) {
	for header, want := range map[string]string{
		"":                          "de",
		"*":                         "de",
		"en":                        "en",
		"en-GB,en;q=0.9":            "en",
		"fr, en-US;q=0.8, de;q=0.5": "en",
		"en;q=0.5, de;q=0.9":        "de",
		"en;q=0, fr":                "de",
		"EN":                        "en",
	} {
		if got := pickLanguage(header); got != want {
			t.Errorf("%q: %s, want %s", header, got, want)
		}
	}
}

// Translated values over HTTP (PL21): Accept-Language picks the language, German without a
// translation, Content-Language says which; Content-Language on a write puts a plain value
// into that language; ?translations=all gives every language; select lists translate too.
func TestTranslationsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	en := map[string]string{"Accept-Language": "en-GB,en;q=0.9"}
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": {"de": "Drucker", "en": "Printer"}}`, nil)
	if rec.code != 201 || rec.body["title"] != "Drucker" || rec.hdr.Get("Content-Language") != "de" {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, admin, http.MethodGet, base, "", en); r.body["title"] != "Printer" || r.hdr.Get("Content-Language") != "en" ||
		!strings.Contains(r.hdr.Get("Vary"), "Accept-Language") {
		t.Fatalf("read in English: %s %v", r.raw, r.hdr)
	}
	if r := call(t, admin, http.MethodGet, base, "", map[string]string{"Accept-Language": "fr"}); r.body["title"] != "Drucker" {
		t.Fatalf("an unknown language reads German: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"?translations=all", "", nil); !strings.Contains(r.raw, `"title":{"de":"Drucker","en":"Printer"}`) {
		t.Fatalf("all languages: %s", r.raw)
	}
	// The list and the JSON query translate as well.
	r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe?filter[title]=Drucker", "", en)
	if !strings.Contains(r.raw, `"title":"Printer"`) {
		t.Fatalf("list in English: %s", r.raw)
	}
	r = call(t, admin, http.MethodPost, "/api/v1/records/probe.probe:query", `{"where": {"field": "title", "eq": "Drucker"}}`, en)
	if r.code != 200 || !strings.Contains(r.raw, `"title":"Printer"`) {
		t.Fatalf("query in English: %d %s", r.code, r.raw)
	}

	// A plain value with Content-Language en changes the English title only.
	upd := call(t, admin, http.MethodPatch, base, `{"title": "Laser printer"}`,
		map[string]string{"If-Match": `"1"`, "Content-Language": "en", "Accept-Language": "en"})
	if upd.code != 200 || upd.body["title"] != "Laser printer" || upd.hdr.Get("ETag") != `"2"` {
		t.Fatalf("update in English: %d %s", upd.code, upd.raw)
	}
	if r := call(t, admin, http.MethodGet, base, "", nil); r.body["title"] != "Drucker" {
		t.Fatalf("German after the English update: %s", r.raw)
	}
	if r := call(t, admin, http.MethodPatch, base, `{"title": "x"}`, map[string]string{"If-Match": `"2"`, "Content-Language": "fr"}); r.code != 422 ||
		!strings.Contains(r.raw, `"code":"language"`) {
		t.Fatalf("write in an unknown language: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/history", "", nil); !strings.Contains(r.raw, `"field":"title.en"`) {
		t.Fatalf("history: %s", r.raw)
	}

	// Select lists: the name of an entry is translatable.
	list := call(t, admin, http.MethodGet, "/api/v1/records/probe.color?filter[key]=red", "", nil)
	items, _ := list.body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("colour red: %s", list.raw)
	}
	red := items[0].(map[string]any)
	entry := "/api/v1/records/probe.color/" + red["id"].(string)
	version := strconv.FormatInt(int64(red["version"].(float64)), 10)
	if r := call(t, admin, http.MethodPatch, entry, `{"name": {"en": "Red"}}`, map[string]string{"If-Match": `"` + version + `"`}); r.code != 200 {
		t.Fatalf("translate a list entry: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, entry, "", en); r.body["name"] != "Red" {
		t.Fatalf("list entry in English: %s", r.raw)
	}
	if r := call(t, admin, http.MethodGet, entry, "", nil); r.body["name"] == "Red" {
		t.Fatalf("list entry in German: %s", r.raw)
	}

	// The OpenAPI describes title as one value or an object per language.
	p := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	title := p.body["components"].(map[string]any)["schemas"].(map[string]any)["probe.probe"].(map[string]any)["properties"].(map[string]any)["title"].(map[string]any)
	if one, _ := title["oneOf"].([]any); len(one) != 2 {
		t.Fatalf("title in the OpenAPI: %v", title)
	}
}
