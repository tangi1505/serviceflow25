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
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD7 (IP28, KF7): a new incident goes to the group responsible for its
// category (sd.responsibility), or for the category above when it has none, and the history
// names the rule; a group named on creating stays. The four queues of the start data are shared with the group "Service Desk": a member
// sees them, and they find the incidents of the person, of the person's groups, without an
// assignee and at risk.
func TestIncidentQueuesOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd7"))
	api := apiServer(t)
	ctx := t.Context()
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 28}, {"../../packages/incident", 40}} {
		p := installPackage(t, ctx, api.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	var desk string
	if err := api.pool.QueryRow(ctx, "SELECT ref FROM package_seed WHERE org_path = 'root' AND package = 'sd' AND key = 'service_desk'").Scan(&desk); err != nil {
		t.Fatal(err)
	}
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	categories := map[string]string{}
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.category?limit=100", "", nil).body["items"].([]any) {
		e := it.(map[string]any)
		categories[fmt.Sprint(e["key"])] = e["id"].(string)
	}

	// The group is responsible for "Netz und Telefonie"; VPN has no responsibility of its own.
	netz := create("core.group", `{"name": "SD7 Netzwerk"}`)
	create("sd.responsibility", `{"category": "`+categories["network"]+`", "group": "`+netz+`"}`)
	const inc = "/api/v1/records/incident.incident"
	groupOf := func(id string) string {
		t.Helper()
		r := call(t, api, http.MethodGet, inc+"/"+id, "", nil)
		list, _ := r.body["actors"].(map[string]any)["group"].([]any)
		if len(list) != 1 {
			return ""
		}
		return list[0].(map[string]any)["id"].(string)
	}
	vpn := create("incident.incident", `{"title": "SD7 VPN bricht ab", "impact": "medium", "urgency": "medium", "category": "`+categories["vpn"]+`"}`)
	if g := groupOf(vpn); g != netz {
		t.Fatalf("group of the VPN incident: %q, want %q", g, netz)
	}
	h := call(t, api, http.MethodGet, inc+"/"+vpn+"/history", "", nil)
	var hist []map[string]any
	_ = json.Unmarshal([]byte(h.raw), &hist)
	if !slices.ContainsFunc(hist, func(e map[string]any) bool {
		return e["field"] == "actors.group" && e["rule"] == "incident_gruppe_nach_kategorie"
	}) {
		t.Fatalf("history: %s", h.raw)
	}
	// A group named on creating stays; a category without a group anywhere gives none.
	own := create("core.group", `{"name": "SD7 Zweite Ebene"}`)
	named := create("incident.incident", `{"title": "SD7 benannt", "impact": "low", "urgency": "low", "category": "`+categories["vpn"]+`",
		"actors": {"group": [{"type": "core.group", "id": "`+own+`"}]}}`)
	if g := groupOf(named); g != own {
		t.Fatalf("named group: %q", g)
	}
	if g := groupOf(create("incident.incident", `{"title": "SD7 Sonstiges", "impact": "low", "urgency": "low", "category": "`+categories["other"]+`"}`)); g != "" {
		t.Fatalf("group without one at the category: %q", g)
	}
	// The rule checks its source: a responsibility has no field "nothing".
	if r := call(t, api, http.MethodPut, "/api/v1/rules/sd7_wrong", `{"type_name": "incident.incident", "trigger": "create", "mode": "before",
		"actions": [{"assign": {"role": "group", "lookup": {"type": "sd.responsibility", "match": "category", "take": "nothing"}}}]}`, nil); r.code != 422 || !strings.Contains(r.raw, "no reference field") {
		t.Fatalf("wrong from: %d %s", r.code, r.raw)
	}

	// Sven works in the service desk and in the network group; he reads incidents and the
	// searches shared with him.
	role := "sd7_agent"
	dropRole(t, api, role)
	if r := call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD7"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{`{"type_name": "incident.incident", "action": "read", "scope": "all"}`,
		`{"type_name": "core.saved_search", "action": "read", "scope": "involved"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	sven := timelineLogin(t, api, "apitest_sd7", role, `{"first_name": "Sven", "last_name": "SD7"}`)
	create("core.group_member", `{"group": "`+desk+`", "person": "`+sven.person+`"}`)
	create("core.group_member", `{"group": "`+netz+`", "person": "`+sven.person+`"}`)
	queues := map[string]string{}
	for tries := 0; len(queues) < 4; tries++ {
		if tries > 50 {
			t.Fatalf("the queues do not reach Sven: %v", queues)
		}
		r := call(t, sven.api, http.MethodGet, "/api/v1/records/core.saved_search?limit=50", "", nil)
		for _, it := range r.body["items"].([]any) {
			s := it.(map[string]any)
			queues[s["name"].(string)] = s["query"].(string)
		}
	}
	find := func(name string) []string {
		t.Helper()
		where, ok := queues[name]
		if !ok {
			t.Fatalf("no queue %q in %v", name, queues)
		}
		r := call(t, sven.api, http.MethodPost, inc+":query", `{"where": `+where+`, "fields": ["title"]}`, nil)
		if r.code != 200 {
			t.Fatalf("queue %s: %d %s", name, r.code, r.raw)
		}
		var titles []string
		for _, it := range r.body["items"].([]any) {
			titles = append(titles, it.(map[string]any)["title"].(string))
		}
		return titles
	}
	if got := find("Störungen meines Teams"); !slices.Equal(got, []string{"SD7 VPN bricht ab"}) {
		t.Fatalf("my team: %v", got)
	}
	if got := find("Störungen ohne Bearbeiter"); len(got) != 3 {
		t.Fatalf("without assignee: %v", got)
	}
	if got := find("Meine Störungen"); len(got) != 0 {
		t.Fatalf("mine before taking one: %v", got)
	}
	v := call(t, api, http.MethodGet, inc+"/"+vpn, "", nil).body["version"]
	if r := call(t, api, http.MethodPatch, inc+"/"+vpn, `{"actors": {"assignee": [{"type": "core.person", "id": "`+sven.person+`"}]}}`,
		map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
		t.Fatalf("assign Sven: %d %s", r.code, r.raw)
	}
	if got := find("Meine Störungen"); !slices.Equal(got, []string{"SD7 VPN bricht ab"}) {
		t.Fatalf("mine: %v", got)
	}
	if got := find("Störungen ohne Bearbeiter"); len(got) != 2 {
		t.Fatalf("without assignee after taking one: %v", got)
	}
	if got := find("Störungen mit SLA-Risiko"); len(got) != 0 {
		t.Fatalf("at risk right after creating: %v", got)
	}
}
