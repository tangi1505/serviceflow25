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
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The acceptance of PL15: a chain of four records, a walk of depth 2 gives exactly two
// levels, a cycle ends; the impact analysis follows depends_on against its direction; the
// rel operator of the query works; audit and events at both ends; rights hide what the
// session may not read.
func TestRelations(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"

	kinds := call(t, admin, http.MethodGet, "/api/v1/relations/kinds", "", nil)
	if kinds.code != 200 || !strings.Contains(kinds.raw, `"core.depends_on"`) && !strings.Contains(kinds.raw, `"name":"depends_on"`) {
		t.Fatalf("kinds: %d %s", kinds.code, kinds.raw)
	}
	start := int64(call(t, admin, http.MethodGet, "/api/v1/events/position", "", nil).body["position"].(float64))
	ids := map[string]string{}
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		r := call(t, admin, http.MethodPost, base, fmt.Sprintf(`{"title": "rel %s"}`, name), nil)
		if r.code != 201 {
			t.Fatalf("record %s: %d %s", name, r.code, r.raw)
		}
		ids[name] = r.body["id"].(string)
	}
	link := func(api *testAPI, kind, from, to string) resp {
		t.Helper()
		return call(t, api, http.MethodPost, "/api/v1/relations", fmt.Sprintf(`{"kind": %q, "from": {"type": "probe.probe", "id": %q}, "to": {"type": "probe.probe", "id": %q}}`, kind, ids[from], ids[to]), nil)
	}
	// The chain A → B → C → D (each depends on the next), E related to B.
	var relAB string
	for _, pair := range [][2]string{{"A", "B"}, {"B", "C"}, {"C", "D"}} {
		r := link(admin, "core.depends_on", pair[0], pair[1])
		if r.code != 201 || r.body["kind"] != "core.depends_on" {
			t.Fatalf("link %v: %d %s", pair, r.code, r.raw)
		}
		if pair[0] == "A" {
			relAB = r.body["id"].(string)
		}
	}
	if r := link(admin, "core.related", "E", "B"); r.code != 201 {
		t.Fatalf("related: %d %s", r.code, r.raw)
	}
	if r := link(admin, "core.nope", "A", "B"); r.code != 422 {
		t.Fatalf("unknown kind: %d %s", r.code, r.raw)
	}
	if r := link(admin, "core.related", "A", "A"); r.code != 422 {
		t.Fatalf("self: %d %s", r.code, r.raw)
	}
	if r := link(admin, "core.depends_on", "A", "B"); r.code != 201 || r.body["id"] != relAB {
		t.Fatalf("linking twice gives the same relation: %d %s", r.code, r.raw)
	}

	graph := func(api *testAPI, path string) (nodes map[string]float64, edges int, r resp) {
		t.Helper()
		r = call(t, api, http.MethodGet, path, "", nil)
		var g struct {
			Nodes []map[string]any `json:"nodes"`
			Edges []map[string]any `json:"edges"`
		}
		_ = json.Unmarshal([]byte(r.raw), &g)
		nodes = map[string]float64{}
		for _, n := range g.Nodes {
			for name, id := range ids {
				if n["id"] == id {
					nodes[name] = n["depth"].(float64)
				}
			}
		}
		return nodes, len(g.Edges), r
	}
	// Depth 2 from A, outgoing: B at 1, C at 2, no D.
	nodes, edges, r := graph(admin, base+"/"+ids["A"]+"/graph?direction=out&depth=2")
	if r.code != 200 || len(nodes) != 3 || nodes["A"] != 0 || nodes["B"] != 1 || nodes["C"] != 2 || edges != 2 {
		t.Fatalf("depth 2: %v %d %s", nodes, edges, r.raw)
	}
	// Both directions from B with all kinds: A, C, E at 1, D at 2.
	nodes, edges, r = graph(admin, base+"/"+ids["B"]+"/graph?depth=3")
	if r.code != 200 || len(nodes) != 5 || nodes["A"] != 1 || nodes["C"] != 1 || nodes["E"] != 1 || nodes["D"] != 2 || edges != 4 {
		t.Fatalf("both from B: %v %d %s", nodes, edges, r.raw)
	}
	// Only depends_on: E is out.
	nodes, _, _ = graph(admin, base+"/"+ids["B"]+"/graph?depth=3&kinds=core.depends_on")
	if _, has := nodes["E"]; has || len(nodes) != 4 {
		t.Fatalf("kinds filter: %v", nodes)
	}
	// A cycle D → A ends: depth 10 from A still gives four nodes and four edges.
	if r := link(admin, "core.depends_on", "D", "A"); r.code != 201 {
		t.Fatalf("cycle: %d %s", r.code, r.raw)
	}
	nodes, edges, r = graph(admin, base+"/"+ids["A"]+"/graph?direction=out&depth=10&kinds=core.depends_on")
	if r.code != 200 || len(nodes) != 4 || edges != 4 || nodes["D"] != 3 {
		t.Fatalf("cycle walk: %v %d", nodes, edges)
	}
	if r := call(t, admin, http.MethodGet, base+"/"+ids["A"]+"/graph?depth=11", "", nil); r.code != 400 && r.code != 422 {
		t.Fatalf("depth over the limit: %d %s", r.code, r.raw)
	}

	// Impact of D: C depends on it (1), B on C (2), A on B (3); E is related only.
	nodes, _, r = graph(admin, base+"/"+ids["D"]+"/impact?depth=5")
	if r.code != 200 || nodes["C"] != 1 || nodes["B"] != 2 || nodes["A"] != 3 {
		t.Fatalf("impact: %v %s", nodes, r.raw)
	}
	if _, has := nodes["E"]; has {
		t.Fatalf("impact must not follow related: %v", nodes)
	}

	// The relations of B: three, filtered by kind and direction.
	r = call(t, admin, http.MethodGet, base+"/"+ids["B"]+"/relations", "", nil)
	var list []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &list)
	if r.code != 200 || len(list) != 3 {
		t.Fatalf("relations of B: %d %s", r.code, r.raw)
	}
	r = call(t, admin, http.MethodGet, base+"/"+ids["B"]+"/relations?direction=out&kind=core.depends_on", "", nil)
	_ = json.Unmarshal([]byte(r.raw), &list)
	if len(list) != 1 || list[0]["to"].(map[string]any)["id"] != ids["C"] {
		t.Fatalf("out of B: %s", r.raw)
	}

	// The rel operator of the query: probes that depend on a record titled "rel C" → B.
	q := call(t, admin, http.MethodPost, base+":query", `{"where": {"rel": "core.depends_on", "to": "probe.probe", "where": {"field": "title", "eq": "rel C"}}}`, nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal([]byte(q.raw), &page)
	if q.code != 200 || len(page.Items) != 1 || page.Items[0]["id"] != ids["B"] {
		t.Fatalf("rel query: %d %s", q.code, q.raw)
	}
	q = call(t, admin, http.MethodPost, base+":query", `{"where": {"rel": "core.depends_on", "to": "probe.probe", "direction": "in", "where": {"field": "title", "starts": "rel A"}}}`, nil)
	_ = json.Unmarshal([]byte(q.raw), &page)
	if q.code != 200 || len(page.Items) != 1 || page.Items[0]["id"] != ids["B"] {
		t.Fatalf("rel query in: %d %s", q.code, q.raw)
	}

	// Audit and events at both ends of A → B.
	for _, name := range []string{"A", "B"} {
		h := call(t, admin, http.MethodGet, base+"/"+ids[name]+"/history", "", nil)
		if h.code != 200 || !strings.Contains(h.raw, `"relate"`) || !strings.Contains(h.raw, `"core.depends_on"`) {
			t.Fatalf("history of %s: %d %s", name, h.code, h.raw)
		}
	}
	// The events of the links: read from the position noted before them; other test
	// packages may have expired events meanwhile, then the problem names the oldest kept one.
	ev := call(t, admin, http.MethodGet, fmt.Sprintf("/api/v1/events?after=%d&limit=1000", start), "", nil)
	if ev.code == 410 {
		var oldest int64
		_, _ = fmt.Sscanf(ev.raw[strings.LastIndex(ev.raw, "event is ")+len("event is "):], "%d", &oldest)
		ev = call(t, admin, http.MethodGet, fmt.Sprintf("/api/v1/events?after=%d&limit=1000", oldest), "", nil)
	}
	if ev.code != 200 || !strings.Contains(ev.raw, `"kind":"relate"`) {
		t.Fatalf("events: %d %s", ev.code, ev.raw[:min(len(ev.raw), 300)])
	}

	// A person who reads only own records sees nothing of the chain, and cannot link it.
	dropRole(t, admin, "rel_own")
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "rel_own"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, action := range []string{"read", "create", "update"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/rel_own/permissions", `{"type_name": "probe.probe", "action": "`+action+`", "scope": "own"}`, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	acc, err := admin.auth.CreateAccount(ctx, "apitest_rel", "apitest_rel", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "apitest_rel"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "rel_own", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_rel", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	own := call(t, user, http.MethodPost, base, `{"title": "rel mine"}`, nil)
	ids["M"] = own.body["id"].(string)
	if r := link(user, "core.related", "M", "A"); r.code != 404 {
		t.Fatalf("linking to an invisible record: %d %s", r.code, r.raw)
	}
	if r := link(admin, "core.related", "M", "A"); r.code != 201 {
		t.Fatalf("admin links M and A: %d %s", r.code, r.raw)
	}
	nodes, edges, r = graph(user, base+"/"+ids["M"]+"/graph?depth=5")
	if r.code != 200 || len(nodes) != 1 || edges != 0 {
		t.Fatalf("graph with own scope hides A: %v %d %s", nodes, edges, r.raw)
	}
	if r := call(t, user, http.MethodGet, base+"/"+ids["A"]+"/graph", "", nil); r.code != 404 {
		t.Fatalf("graph of an invisible record: %d", r.code)
	}

	// Delete: the relation is gone, unrelate is in the history, the records stay.
	if r := call(t, admin, http.MethodDelete, "/api/v1/relations/"+relAB, "", nil); r.code != 204 {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/relations/"+relAB, "", nil); r.code != 404 {
		t.Fatalf("deleted relation: %d", r.code)
	}
	if h := call(t, admin, http.MethodGet, base+"/"+ids["A"]+"/history", "", nil); !strings.Contains(h.raw, `"unrelate"`) {
		t.Fatalf("unrelate in history: %s", h.raw)
	}
	nodes, _, _ = graph(admin, base+"/"+ids["A"]+"/graph?direction=out&depth=2&kinds=core.depends_on")
	if _, has := nodes["B"]; has {
		t.Fatalf("B must be unreachable from A now: %v", nodes)
	}
}
