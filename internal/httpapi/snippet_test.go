// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// snippetWhere is the condition of the editor's list (web/src/render/Snippets.tsx): the use,
// the type or none, shared with the person, their groups or nobody.
func snippetWhere(use, typeName string) string {
	return `{"and": [{"field": "use", "eq": "` + use + `"},
		{"or": [{"field": "target_type", "null": true}, {"field": "target_type", "eq": "` + typeName + `"}]},
		{"or": [{"field": "actors.shared", "null": true}, {"field": "actors.shared", "eq": "@my_groups"}, {"field": "actors.shared", "eq": "@me"}]}]}`
}

// The Prüfweg of SD8 (Inventar G56): the text module "Protokoll anfordern" of the start data
// comes filled in with the title of the incident and the name of the one who writes; shared
// with the service desk, only its members find it; a wrong placeholder is refused on saving;
// a reference shows its title, a choice its label, {{link}} a link, and the values are escaped.
func TestSnippetsOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd8"))
	api := apiServer(t)
	ctx := t.Context()
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 28}, {"../../packages/incident", 39}} {
		p := installPackage(t, ctx, api.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	desk := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/core.group?limit=100", "", nil).body["items"].([]any) {
		if g := it.(map[string]any); g["name"] == "Service Desk" {
			desk = g["id"].(string)
		}
	}
	network := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.category?limit=100", "", nil).body["items"].([]any) {
		if c := it.(map[string]any); c["key"] == "network" {
			network = c["id"].(string)
		}
	}
	if desk == "" || network == "" {
		t.Fatalf("service desk %q, category network %q", desk, network)
	}

	// Lena works in the service desk, Olaf not; both read incidents and text modules.
	role := "sd8_agent"
	dropRole(t, api, role)
	if r := call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD8"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{`{"type_name": "incident.incident", "action": "read", "scope": "all"}`,
		`{"type_name": "core.snippet", "action": "read", "scope": "all"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	lena := timelineLogin(t, api, "apitest_sd8_lena", role, `{"first_name": "Lena", "last_name": "SD8"}`)
	olaf := timelineLogin(t, api, "apitest_sd8_olaf", role, `{"first_name": "Olaf", "last_name": "SD8"}`)
	create("core.group_member", `{"group": "`+desk+`", "person": "`+lena.person+`"}`)

	const inc = "/api/v1/records/incident.incident"
	id := create("incident.incident", `{"title": "SD8 Drucker & <Fax>", "impact": "low", "urgency": "low", "category": "`+network+`"}`)
	offered := func(who *testAPI, use string) map[string]string {
		t.Helper()
		r := call(t, who, http.MethodPost, "/api/v1/records/core.snippet:query", `{"where": `+snippetWhere(use, "incident.incident")+`, "fields": ["name"]}`, nil)
		if r.code != 200 {
			t.Fatalf("snippets: %d %s", r.code, r.raw)
		}
		out := map[string]string{}
		for _, it := range r.body["items"].([]any) {
			s := it.(map[string]any)
			out[s["name"].(string)] = s["id"].(string)
		}
		return out
	}
	// The membership reaches the access after its cache (WF3), so Lena may need a moment.
	var mine map[string]string
	for tries := 0; ; tries++ {
		if mine = offered(lena.api, "answer"); mine["Protokoll anfordern"] != "" || tries > 50 {
			break
		}
	}
	if _, ok := mine["Protokoll anfordern"]; !ok {
		t.Fatalf("Lena does not find the text module of her group: %v", mine)
	}
	if got := offered(olaf.api, "answer"); len(got) != 0 {
		t.Fatalf("Olaf finds the text module of a group he is not in: %v", got)
	}
	solutions := offered(olaf.api, "solution")
	if _, ok := solutions["Gelöst durch Neustart"]; !ok || len(solutions) != 1 {
		t.Fatalf("solutions for all: %v", solutions)
	}

	// Filled in for Lena: the title escaped, her name as {{actor}}.
	r := call(t, lena.api, http.MethodPost, inc+"/"+id+":snippet", `{"snippet": "`+mine["Protokoll anfordern"]+`"}`, nil)
	if r.code != 200 {
		t.Fatalf("render: %d %s", r.code, r.raw)
	}
	body, _ := r.body["body"].(string)
	if !strings.Contains(body, "„SD8 Drucker &amp; &lt;Fax&gt;“") || !strings.Contains(body, "Lena SD8") || strings.Contains(body, "{{") {
		t.Fatalf("filled in: %s", body)
	}

	// A reference by its title, a choice by its label, the link as a link; nothing is written.
	own := create("core.snippet", `{"name": "SD8 Alles", "use": "answer", "target_type": "incident.incident",
		"body": "<p>{{record.category}} / {{record.impact}} / {{type}} / {{link}} / {{record.number}}</p>"}`)
	r = call(t, lena.api, http.MethodPost, inc+"/"+id+":snippet", `{"snippet": "`+own+`"}`, nil)
	body, _ = r.body["body"].(string)
	number := call(t, api, http.MethodGet, inc+"/"+id, "", nil).body["number"]
	for _, want := range []string{"Netz und Telefonie", "Niedrig", "Störung", `<a href="http://itsm.test/types/incident.incident/` + id + `">`, "INC-"} {
		if r.code != 200 || !strings.Contains(body, want) {
			t.Fatalf("want %q in %d %s (number %v)", want, r.code, body, number)
		}
	}
	if v := call(t, api, http.MethodGet, inc+"/"+id, "", nil).body["version"]; v != float64(1) {
		t.Fatalf("rendering wrote the incident: version %v", v)
	}

	// A text module for incidents does not fit a person.
	person := create("core.person", `{"first_name": "Paul", "last_name": "SD8"}`)
	if r := call(t, api, http.MethodPost, "/api/v1/records/core.person/"+person+":snippet", `{"snippet": "`+solutions["Gelöst durch Neustart"]+`"}`, nil); r.code != 422 {
		t.Fatalf("other type: %d %s", r.code, r.raw)
	}

	// Saving checks the placeholders: a field the type lacks, one of the mails only, a type
	// not installed; without a type, a field of some type is fine.
	for body, want := range map[string]string{
		`{"name": "SD8 x", "use": "answer", "target_type": "incident.incident", "body": "<p>{{record.nope}}</p>"}`: `has no field \"nope\"`,
		`{"name": "SD8 x", "use": "answer", "body": "<p>{{entry}}</p>"}`:                                           "is no placeholder",
		`{"name": "SD8 x", "use": "answer", "target_type": "nope.nope", "body": "<p>x</p>"}`:                       "is not installed",
		`{"name": "SD8 x", "use": "answer", "body": "<p>{{record.nirgends}}</p>"}`:                                 "no installed type has a field",
	} {
		if r := call(t, api, http.MethodPost, "/api/v1/records/core.snippet", body, nil); r.code != 422 || !strings.Contains(r.raw, want) {
			t.Fatalf("%s: %d %s, want %s", body, r.code, r.raw, want)
		}
	}
	create("core.snippet", `{"name": "SD8 ohne Typ", "use": "answer", "body": "<p>{{record.urgency}} {{title}}</p>"}`)
	if got := offered(olaf.api, "answer"); got["SD8 ohne Typ"] == "" || got["SD8 Alles"] == "" || len(got) != 2 {
		t.Fatalf("modules for all: %v", got)
	}
}
