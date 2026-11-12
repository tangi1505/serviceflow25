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
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD13 (SD-Plan E15): an offer only for the Dienststelle Nord is in the catalog
// there and nowhere else; ordering it makes a request with the answers, the offer's template
// (so the approval is asked), its delivery time as the deadline and its costs, linked to the
// offer; in English the offer reads English. What cannot be ordered is refused.
func TestCatalogOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd13"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 28}, {"../../packages/request", 12}, {"../../packages/catalog", 4}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	const offers = "/api/v1/records/catalog.offer"
	const requests = "/api/v1/records/request.request"

	// The type is an area of the portal with facts, and orderable.
	ty := call(t, admin, http.MethodGet, "/api/v1/types/catalog.offer", "", nil)
	if ty.code != 200 || !strings.Contains(ty.raw, `"facts":["delivery_days","cost","cost_kind"]`) || !strings.Contains(ty.raw, `"relation":"catalog.ordered"`) {
		t.Fatalf("type: %d %.800s", ty.code, ty.raw)
	}

	for _, n := range []string{"nord", "sued"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "`+n+`"}`, nil); r.code != 201 {
			t.Fatalf("node %s: %d %s", n, r.code, r.raw)
		}
	}
	// Persons of the portal: they create requests and read their own; the offers they read by
	// the readers of the type.
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd13_user", "label": {"de": "SD13"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{`{"type_name": "request.request", "action": "create", "scope": "all"}`, `{"type_name": "request.request", "action": "read", "scope": "involved"}`} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/sd13_user/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	nord := timelineLogin(t, admin, "apitest_sd13_nord", "sd13_user", `{"last_name": "Neu", "org_path": "root.nord"}`)
	sued := timelineLogin(t, admin, "apitest_sd13_sued", "sd13_user", `{"last_name": "Süd", "org_path": "root.sued"}`)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "sd13_lead", "label": {"de": "SD13 Leitung"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	lead := timelineLogin(t, admin, "apitest_sd13_lead", "sd13_lead", `{"last_name": "Leitung"}`)

	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, admin, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	files := create("core.group", `{"name": "SD13 Fileserver"}`)
	// Since WF7 the stages are an approval template the fulfilment template names.
	approval := create("core.approval_template", `{"name": "Zugang Gruppenlaufwerk"}`)
	create("core.approval_template_stage", `{"template": "`+approval+`", "name": "Fachliche Prüfung", "position": 10,
		"actors": {"approvers": [{"type": "core.person", "id": "`+lead.person+`"}]}}`)
	tpl := create("request.template", `{"name": "Zugang Gruppenlaufwerk", "approval_template": "`+approval+`"}`)
	create("request.template_task", `{"template": "`+tpl+`", "title": "Rechte setzen", "group": "`+files+`", "position": 10}`)
	form := create("core.form", `{"name": "Zugang Gruppenlaufwerk bestellen", "target_type": "request.request", "active": true, "place": "order",
		"definition": "{\"sections\": [{\"id\": \"zugang\", \"title\": {\"de\": \"Zugang\"}, \"questions\": [{\"id\": \"laufwerk\", \"type\": \"text\", \"required\": true, \"label\": {\"de\": \"Laufwerk\", \"en\": \"Drive\"}}, {\"id\": \"rechte\", \"type\": \"choice\", \"label\": {\"de\": \"Rechte\"}, \"options\": [{\"value\": \"r\", \"label\": {\"de\": \"Lesen\"}}, {\"value\": \"rw\", \"label\": {\"de\": \"Lesen und Schreiben\"}}]}]}]}"}`)
	drives := create("catalog.offer", `{"name": "Zugang Gruppenlaufwerk", "description": "<p>Rechte auf ein Laufwerk der Dienststelle.</p>",
		"area": "`+listEntry(t, admin, "catalog.area", "drives")+`", "form": "`+form+`", "template": "`+tpl+`",
		"delivery_days": 3, "cost": 0, "cost_kind": "once", "org_path": "root.nord"}`)
	if r := call(t, admin, http.MethodPatch, offers+"/"+drives, `{"name": {"en": "Access to a group drive"}}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("English name: %d %s", r.code, r.raw)
	}

	// Only Nord has the offer for Nord; both have the example of the start data.
	names := func(u timelineUser) []string {
		t.Helper()
		r := call(t, u.api, http.MethodGet, offers+"?limit=50", "", nil)
		if r.code != 200 {
			t.Fatalf("offers: %d %s", r.code, r.raw)
		}
		var out []string
		for _, it := range r.body["items"].([]any) {
			out = append(out, it.(map[string]any)["name"].(string))
		}
		slices.Sort(out)
		return out
	}
	if got := names(nord); !slices.Equal(got, []string{"Neuer Arbeitsplatz", "Zugang Gruppenlaufwerk"}) {
		t.Fatalf("Nord sees %v", got)
	}
	if got := names(sued); !slices.Equal(got, []string{"Neuer Arbeitsplatz"}) {
		t.Fatalf("Süd sees %v", got)
	}
	if r := call(t, nord.api, http.MethodGet, offers+"/"+drives, "", map[string]string{"Accept-Language": "en"}); r.code != 200 || r.body["name"] != "Access to a group drive" {
		t.Fatalf("Nord reads in English: %d %s", r.code, r.raw)
	}
	// The form of an offer is not offered on its own.
	if r := call(t, nord.api, http.MethodGet, "/api/v1/forms", "", nil); r.code != 200 || strings.Contains(r.raw, "bestellen") {
		t.Fatalf("forms of the portal: %d %s", r.code, r.raw)
	}

	// Süd cannot order what it does not see; an answer missing is named by its question.
	order := func(u timelineUser, id, body string) resp {
		t.Helper()
		return call(t, u.api, http.MethodPost, offers+"/"+id+":order", body, nil)
	}
	if r := order(sued, drives, `{"answers": {"laufwerk": "N:\\Nord"}}`); r.code != 404 {
		t.Fatalf("Süd orders the offer for Nord: %d %s", r.code, r.raw)
	}
	if r := order(nord, drives, `{"answers": {}}`); r.code != 422 || !strings.Contains(r.raw, `"laufwerk"`) {
		t.Fatalf("without the drive: %d %s", r.code, r.raw)
	}

	// Ordered: a request with the answers, the offer's values, linked to the offer.
	r := order(nord, drives, `{"answers": {"laufwerk": "N:\\Nord\\Einsatz", "rechte": "rw"}}`)
	if r.code != 201 {
		t.Fatalf("order: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	if r.body["title"] != "Zugang Gruppenlaufwerk" || r.body["template"] != tpl || r.body["delivery_days"] != float64(3) ||
		r.body["cost"] != float64(0) || r.body["cost_kind"] != "once" || r.body["org_path"] != "root" {
		t.Fatalf("the request: %v", r.body)
	}
	holders, _ := r.body["actors"].(map[string]any)["requester"].([]any)
	if len(holders) != 1 || holders[0].(map[string]any)["id"] != nord.person {
		t.Fatalf("requester: %v", r.body["actors"])
	}
	if bodies := entryBodies(timeline(t, admin, requests+"/"+id)); len(bodies) != 1 || !strings.Contains(bodies[0], `Laufwerk: N:\Nord\Einsatz`) || !strings.Contains(bodies[0], "Rechte: Lesen und Schreiben") {
		t.Fatalf("the answers: %v", bodies)
	}
	rels := call(t, admin, http.MethodGet, requests+"/"+id+"/relations", "", nil)
	if rels.code != 200 || !strings.Contains(rels.raw, `"kind":"catalog.ordered"`) || !strings.Contains(rels.raw, drives) {
		t.Fatalf("relations: %d %s", rels.code, rels.raw)
	}
	// The deadline of the fulfilment is the delivery time: three days of the service time.
	clocks := call(t, admin, http.MethodGet, requests+"/"+id+"/clocks", "", nil)
	var views []map[string]any
	if clocks.code != 200 || json.Unmarshal([]byte(clocks.raw), &views) != nil {
		t.Fatalf("clocks: %d %s", clocks.code, clocks.raw)
	}
	var fulfilment map[string]any
	for _, v := range views {
		if strings.Contains(clocks.raw, "Erfüllung") && v["target_minutes"] == float64(3*660) {
			fulfilment = v
		}
	}
	if fulfilment == nil {
		t.Fatalf("no clock of three days: %s", clocks.raw)
	}
	// The template of the offer asks for the approval, as with any request (SD6).
	deadline := time.Now().Add(20 * time.Second)
	for {
		for _, d := range admin.rt.Databases() {
			if err := admin.ruleEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
		}
		if g := call(t, admin, http.MethodGet, requests+"/"+id, "", nil); g.body["state"] == "approval" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waited in vain for the approval of the offer's template")
		}
		time.Sleep(200 * time.Millisecond)
	}
	// The requester reads it in the portal.
	if g := call(t, nord.api, http.MethodGet, requests+"/"+id, "", nil); g.code != 200 {
		t.Fatalf("the requester reads the request: %d %s", g.code, g.raw)
	}

	// Without a delivery time five days, as before the catalog.
	plain := create("request.request", `{"title": "Auskunft"}`)
	if c := call(t, admin, http.MethodGet, requests+"/"+plain+"/clocks", "", nil); !strings.Contains(c.raw, `"target_minutes":3300`) {
		t.Fatalf("clock without an offer: %s", c.raw)
	}

	// What cannot be ordered: a type without order, an offer without a form.
	if r := call(t, admin, http.MethodPost, "/api/v1/records/request.template/"+tpl+":order", `{"answers": {}}`, nil); r.code != 404 {
		t.Fatalf("order a template: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, offers, `{"name": "Ohne", "description": "<p>x</p>", "area": "`+listEntry(t, admin, "catalog.area", "drives")+`"}`, nil); r.code != 422 || !strings.Contains(r.raw, `"form"`) {
		t.Fatalf("an offer without a form: %d %s", r.code, r.raw)
	}
	// A target field that is no number field of the type is refused.
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.sla", `{"name": "Falsch", "target_type": "request.request", "target_minutes": 60, "target_field": "title", "stop": "true"}`, nil); r.code != 422 || !strings.Contains(r.raw, "target_field") {
		t.Fatalf("a target field of text: %d %s", r.code, r.raw)
	}
}

// listEntry is the id of the entry of a select list with the key.
func listEntry(t *testing.T, admin *testAPI, typeName, key string) string {
	t.Helper()
	r := call(t, admin, http.MethodPost, "/api/v1/records/"+typeName+":query", `{"where": {"field": "key", "eq": "`+key+`"}}`, nil)
	items, _ := r.body["items"].([]any)
	if r.code != 200 || len(items) != 1 {
		t.Fatalf("entry %s of %s: %d %s", key, typeName, r.code, r.raw)
	}
	return items[0].(map[string]any)["id"].(string)
}
