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
	"strings"
	"testing"

	"github.com/google/uuid"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD18 (SD-Plan E12, N13 to N16): while a user reports a fault, the articles
// she may read come as suggestions, any word counting; she says whether one helped and
// comments on it; an agent links an article to an incident he may change without the right to
// change the article, and the suggestion then shows the link and how often it was used.
func TestKnowledgeAtTicketOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd18"))
	admin := apiServer(t)
	ctx := t.Context()
	starter := seed.New(admin.rt, records.New(admin.rt), admin.rules)
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 30}, {"../../packages/incident", 58}, {"../../packages/knowledge", 10}} {
		p := installPackage(t, ctx, admin.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
	}
	const arts = "/api/v1/records/knowledge.article"
	const incidents = "/api/v1/records/incident.incident"
	for _, role := range []string{"sd18_agent", "sd18_user"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD18"}}`, nil); r.code != 201 {
			t.Fatalf("role: %d %s", r.code, r.raw)
		}
	}
	for _, p := range []string{
		`{"type_name": "incident.incident", "action": "*", "scope": "all"}`,
		`{"type_name": "knowledge.article", "action": "read", "scope": "all"}`,
	} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/sd18_agent/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	agent := timelineLogin(t, admin, "apitest_sd18_agent", "sd18_agent", `{"last_name": "Bearbeiter"}`)
	user := timelineLogin(t, admin, "apitest_sd18_user", "sd18_user", `{"last_name": "Nutzerin"}`)
	// Since SD17 an article is published once an editor approved it.
	editor := timelineLogin(t, admin, "apitest_sd18_editor", "sd18_user", `{"last_name": "Redaktion"}`)
	article := func(title, body string, publish bool) string {
		t.Helper()
		r := call(t, admin, http.MethodPost, arts, `{"title": "`+title+`", "body": "<p>`+body+`</p>", "audience": "all"}`, nil)
		if r.code != 201 {
			t.Fatalf("article %s: %d %s", title, r.code, r.raw)
		}
		id := r.body["id"].(string)
		if publish {
			publishArticle(t, admin, admin, editor, arts, id)
		}
		return id
	}
	vpn := article("VPN verbindet nicht: erste Schritte", "Bricht die Verbindung ab, starten Sie den VPN-Client neu.", true)
	printer := article("Drucker druckt nicht", "Legen Sie Papier nach.", true)
	draft := article("VPN im Entwurf", "Noch nicht fertig: VPN bricht ab.", false)

	type hit struct {
		ID      string          `json:"id"`
		Title   string          `json:"title"`
		Snippet string          `json:"snippet"`
		Rating  *records.Rating `json:"rating"`
		Uses    *int64          `json:"uses"`
		Linked  *string         `json:"linked"`
	}
	type group struct {
		Type     string            `json:"type"`
		Label    map[string]string `json:"label"`
		Relation string            `json:"relation"`
		Apply    *struct {
			Label map[string]string `json:"label"`
			Set   map[string]any    `json:"set"`
		} `json:"apply"`
		Hits []hit `json:"hits"`
	}
	suggestions := func(api *testAPI, body string) group {
		t.Helper()
		r := call(t, api, http.MethodPost, incidents+":suggest", body, nil)
		var gs []group
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &gs) != nil || len(gs) != 1 {
			t.Fatalf("suggest %s: %d %s", body, r.code, r.raw)
		}
		return gs[0]
	}
	ids := func(g group) []string {
		var out []string
		for _, h := range g.Hits {
			out = append(out, h.ID)
		}
		return out
	}

	// The form of the portal: any word counts, the draft is not hers to read.
	form := `{"values": {"title": "VPN bricht ab", "description": "<p>Seit heute früh bricht die Verbindung nach ein paar Minuten ab.</p>"}, "limit": 3}`
	g := suggestions(user.api, form)
	if g.Type != "knowledge.article" || g.Label["de"] != "Wissen" || g.Relation != "knowledge.uses" {
		t.Fatalf("the group: %+v", g)
	}
	if got := ids(g); len(got) != 1 || got[0] != vpn {
		t.Fatalf("the user is offered %v, want only the published VPN article %s (not the draft %s)", got, vpn, draft)
	}
	if h := g.Hits[0]; !strings.Contains(h.Snippet, "<mark>") || h.Rating == nil || h.Rating.Yes != 0 || h.Uses == nil || *h.Uses != 0 || h.Linked != nil {
		t.Fatalf("the hit: %+v", h)
	}
	// Using it sets the text and the kind "Anleitung gegeben" by the id of its entry.
	if g.Apply == nil || g.Apply.Set["solution"] != "@body" || g.Apply.Label["de"] != "Als Lösung übernehmen" {
		t.Fatalf("apply: %+v", g.Apply)
	}
	if kind, _ := g.Apply.Set["solution_kind"].(string); uuid.Validate(kind) != nil {
		t.Fatalf("the kind of solution is no id: %v", g.Apply.Set["solution_kind"])
	}
	if got := ids(suggestions(user.api, `{"text": "Drucker"}`)); len(got) != 1 || got[0] != printer {
		t.Fatalf("an own search finds %v, want the printer %s", got, printer)
	}
	if got := ids(suggestions(user.api, `{"values": {}}`)); len(got) != 0 {
		t.Fatalf("an empty form is offered %v", got)
	}
	if r := call(t, user.api, http.MethodPost, incidents+":suggest", `{"text": "`+strings.Repeat("x", 201)+`"}`, nil); r.code != 422 {
		t.Fatalf("a long own search: %d %s", r.code, r.raw)
	}

	// Helped: once per person, changeable, taken back with null; the draft is not hers.
	rate := func(api *testAPI, id, vote string, want int) records.Rating {
		t.Helper()
		r := call(t, api, http.MethodPut, arts+"/"+id+"/rating", `{"helpful": `+vote+`}`, nil)
		if r.code != want {
			t.Fatalf("rate %s with %s: %d %s", id, vote, r.code, r.raw)
		}
		var sum records.Rating
		_ = json.Unmarshal([]byte(r.raw), &sum)
		return sum
	}
	if sum := rate(user.api, vpn, "true", 200); sum.Yes != 1 || sum.No != 0 || sum.Mine == nil || !*sum.Mine {
		t.Fatalf("after yes: %+v", sum)
	}
	if sum := rate(user.api, vpn, "false", 200); sum.Yes != 0 || sum.No != 1 || sum.Mine == nil || *sum.Mine {
		t.Fatalf("after changing to no: %+v", sum)
	}
	if sum := rate(agent.api, vpn, "true", 200); sum.Yes != 1 || sum.No != 1 || !*sum.Mine {
		t.Fatalf("the agent's yes: %+v", sum)
	}
	if sum := rate(agent.api, vpn, "null", 200); sum.Yes != 0 || sum.No != 1 || sum.Mine != nil {
		t.Fatalf("the agent takes it back: %+v", sum)
	}
	rate(user.api, draft, "true", 404)
	if r := call(t, user.api, http.MethodPut, arts+"/"+vpn+"/rating", `{}`, nil); r.code != 422 {
		t.Fatalf("a vote without helpful: %d %s", r.code, r.raw)
	}
	if r := call(t, user.api, http.MethodPut, incidents+"/"+uuid.NewString()+"/rating", `{"helpful": true}`, nil); r.code != 404 {
		t.Fatalf("a type without the trait rating: %d %s", r.code, r.raw)
	}

	// Comments: whom a reader rule lets comment writes public entries, no private ones.
	if r := call(t, user.api, http.MethodPost, arts+"/"+vpn+"/timeline", `{"body": "Schritt 2 hat gereicht."}`, nil); r.code != 201 {
		t.Fatalf("the user comments: %d %s", r.code, r.raw)
	}
	if r := call(t, user.api, http.MethodPost, arts+"/"+vpn+"/timeline", `{"body": "Nur intern", "private": true}`, nil); r.code != 403 {
		t.Fatalf("a private comment of the user: %d %s", r.code, r.raw)
	}
	if r := call(t, user.api, http.MethodPost, arts+"/"+draft+"/timeline", `{"body": "Zum Entwurf"}`, nil); r.code != 404 {
		t.Fatalf("a comment on the draft: %d %s", r.code, r.raw)
	}
	found := false
	for _, it := range timeline(t, agent.api, arts+"/"+vpn) {
		if e, _ := it["entry"].(map[string]any); e != nil && strings.Contains(e["body"].(string), "Schritt 2") {
			found = true
		}
	}
	if !found {
		t.Fatal("the agent does not see the comment in the timeline")
	}

	// Linking: the agent changes the incident and only reads the article; the user may not.
	inc := call(t, agent.api, http.MethodPost, incidents, `{"title": "VPN bricht ab", "description": "<p>Seit heute bricht die Verbindung ab.</p>"}`, nil)
	if inc.code != 201 {
		t.Fatalf("incident: %d %s", inc.code, inc.raw)
	}
	incID := inc.body["id"].(string)
	link := `{"kind": "knowledge.uses", "from": {"type": "incident.incident", "id": "` + incID + `"}, "to": {"type": "knowledge.article", "id": "` + vpn + `"}}`
	if r := call(t, user.api, http.MethodPost, "/api/v1/relations", link, nil); r.code != 404 && r.code != 403 {
		t.Fatalf("the user links: %d %s", r.code, r.raw)
	}
	rel := call(t, agent.api, http.MethodPost, "/api/v1/relations", link, nil)
	if rel.code != 201 {
		t.Fatalf("the agent links: %d %s", rel.code, rel.raw)
	}
	g = suggestions(agent.api, `{"values": {"title": "VPN bricht ab"}, "id": "`+incID+`"}`)
	if len(g.Hits) == 0 || g.Hits[0].ID != vpn || g.Hits[0].Linked == nil || *g.Hits[0].Linked != rel.body["id"] || *g.Hits[0].Uses != 1 {
		t.Fatalf("after linking: %+v", g.Hits)
	}
	if r := call(t, agent.api, http.MethodDelete, "/api/v1/relations/"+rel.body["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("the agent unlinks: %d %s", r.code, r.raw)
	}
	if g := suggestions(agent.api, `{"values": {"title": "VPN bricht ab"}, "id": "`+incID+`"}`); g.Hits[0].Linked != nil || *g.Hits[0].Uses != 0 {
		t.Fatalf("after unlinking: %+v", g.Hits[0])
	}
}
