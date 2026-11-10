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
	"time"

	"group24.de/itsm/internal/db/dbtest"
)

// The Prüfweg of WF6 (WF-Plan E17): two stages, stage 2 only after stage 1; a stage of three
// with "at least 2" is given after the second yes, also when one refused before; a substitute
// decides for the person she stands in for; a refusal needs a reason, closes the order through
// its transition and leaves the reason in the timeline; whoever is asked reads the record
// without a grant, nobody else; a withdrawn approval drops the questions.
func TestApprovalsOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_wf6"))
	admin := apiServer(t)
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "wf6_user", "label": {"de": "WF6"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	lead := timelineLogin(t, admin, "apitest_wf6_lead", "wf6_user", `{"last_name": "Leitung"}`)
	sub := timelineLogin(t, admin, "apitest_wf6_sub", "wf6_user", `{"last_name": "Vertreter"}`)
	sec1 := timelineLogin(t, admin, "apitest_wf6_sec1", "wf6_user", `{"last_name": "Sicher"}`)
	member := timelineLogin(t, admin, "apitest_wf6_member", "wf6_user", `{"last_name": "Muster"}`)
	sec2 := timelineLogin(t, admin, "apitest_wf6_sec2", "wf6_user", `{"last_name": "Prüfer"}`)
	other := timelineLogin(t, admin, "apitest_wf6_other", "wf6_user", `{"last_name": "Andere"}`)
	group := call(t, admin, http.MethodPost, "/api/v1/records/core.group", `{"name": "WF6 Informationssicherheit"}`, nil)
	if group.code != 201 {
		t.Fatalf("group: %d %s", group.code, group.raw)
	}
	gid := group.body["id"].(string)
	if r := call(t, admin, http.MethodPost, "/api/v1/records/core.group_member", `{"group": "`+gid+`", "person": "`+member.person+`"}`, nil); r.code != 201 {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}

	const orders = "/api/v1/records/probe.order"
	newOrder := func(title string) string {
		t.Helper()
		r := call(t, admin, http.MethodPost, orders, `{"title": "`+title+`"}`, nil)
		if r.code != 201 {
			t.Fatalf("order: %d %s", r.code, r.raw)
		}
		if r.body["approval"] != nil {
			t.Fatalf("a new order has no approval yet, got %v", r.body["approval"])
		}
		return orders + "/" + r.body["id"].(string)
	}
	person := func(id string) string { return `{"type": "core.person", "id": "` + id + `"}` }
	type step struct {
		ID        string  `json:"id"`
		State     string  `json:"state"`
		Comment   string  `json:"comment"`
		DecidedBy *string `json:"decided_by"`
		OnBehalf  *string `json:"on_behalf"`
		Mine      bool    `json:"mine"`
	}
	type approval struct {
		ID     string `json:"id"`
		State  string `json:"state"`
		Stage  int    `json:"stage"`
		Stages []struct {
			State string `json:"state"`
			Steps []step `json:"steps"`
		} `json:"stages"`
	}
	read := func(api *testAPI, url string) approval {
		t.Helper()
		r := call(t, api, http.MethodGet, url+"/approvals", "", nil)
		var list []approval
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil || len(list) == 0 {
			t.Fatalf("approvals: %d %s", r.code, r.raw)
		}
		return list[0]
	}
	field := func(url string) (string, string) {
		t.Helper()
		r := call(t, admin, http.MethodGet, url, "", nil)
		if r.code != 200 {
			t.Fatalf("order: %d %s", r.code, r.raw)
		}
		f, _ := r.body["approval"].(string)
		return r.body["state"].(string), f
	}
	mine := func(api *testAPI, state string) []map[string]any {
		t.Helper()
		r := call(t, api, http.MethodGet, "/api/v1/me/approvals?state="+state, "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("my approvals: %d %s", r.code, r.raw)
		}
		return list
	}
	decide := func(api *testAPI, url, id, body string, want int) {
		t.Helper()
		if r := call(t, api, http.MethodPost, url+"/approvals/"+id+":decide", body, nil); r.code != want {
			t.Fatalf("decide %s: %d %s, want %d", body, r.code, r.raw, want)
		}
	}

	// Asking: two stages; the order waits for approval.
	order := newOrder("Zugang Gruppenlaufwerk Nord")
	request := `{"message": "Bitte bis Freitag", "stages": [
		{"name": "Fachliche Prüfung", "approvers": [` + person(lead.person) + `]},
		{"name": "Informationssicherheit", "need": 2, "approvers": [` + person(sec1.person) + `, {"type": "core.group", "id": "` + gid + `"}, ` + person(sec2.person) + `]}]}`
	created := call(t, admin, http.MethodPost, order+"/approvals", request, nil)
	if created.code != 201 {
		t.Fatalf("request: %d %s", created.code, created.raw)
	}
	aid := created.body["id"].(string)
	if state, f := field(order); state != "waiting" || f != "pending" {
		t.Fatalf("after asking: state %s, approval %s", state, f)
	}
	if r := call(t, admin, http.MethodPost, order+"/approvals", request, nil); r.code != 409 {
		t.Fatalf("a second open approval: %d %s", r.code, r.raw)
	}

	// Who reads: the asked of stage 1, not those of stage 2 yet, not the others.
	if r := call(t, lead.api, http.MethodGet, order, "", nil); r.code != 200 || r.body["title"] != "Zugang Gruppenlaufwerk Nord" {
		t.Fatalf("the asked lead reads: %d %s", r.code, r.raw)
	}
	for name, u := range map[string]timelineUser{"sec1": sec1, "other": other, "sub": sub} {
		if r := call(t, u.api, http.MethodGet, order, "", nil); r.code != 404 {
			t.Fatalf("%s reads before being asked: %d", name, r.code)
		}
	}
	if list := mine(lead.api, "open"); len(list) != 1 || list[0]["title"] != "Zugang Gruppenlaufwerk Nord" || list[0]["stage_name"] != "Fachliche Prüfung" {
		t.Fatalf("Zu genehmigen of the lead: %v", list)
	}
	if list := mine(sec1.api, "open"); len(list) != 0 {
		t.Fatalf("stage 2 asks before stage 1: %v", list)
	}
	if a := read(lead.api, order); !a.Stages[0].Steps[0].Mine || a.Stages[1].State != "waiting" {
		t.Fatalf("the lead's view: %+v", a)
	}
	decide(sec1.api, order, aid, `{"decision": "approve"}`, 404)

	// The lead names a substitute; the substitute decides for her.
	if r := call(t, other.api, http.MethodPost, "/api/v1/persons/"+lead.person+"/substitutes", `{"substitute": "`+other.person+`"}`, nil); r.code != 404 && r.code != 403 {
		t.Fatalf("someone else names the lead's substitute: %d %s", r.code, r.raw)
	}
	if r := call(t, lead.api, http.MethodPost, "/api/v1/persons/"+lead.person+"/substitutes", `{"substitute": "`+lead.person+`"}`, nil); r.code != 422 {
		t.Fatalf("herself as substitute: %d %s", r.code, r.raw)
	}
	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if r := call(t, lead.api, http.MethodPost, "/api/v1/persons/"+lead.person+"/substitutes",
		`{"substitute": "`+sub.person+`", "valid_from": "`+from+`", "valid_until": "`+until+`"}`, nil); r.code != 201 || r.body["active"] != true {
		t.Fatalf("substitute: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/persons/"+lead.person+"/substitutes", "", nil); r.code != 200 || !strings.Contains(r.raw, sub.person) {
		t.Fatalf("the administration reads the substitutes: %d %s", r.code, r.raw)
	}
	if list := mine(sub.api, "open"); len(list) != 1 || list[0]["on_behalf"] != lead.person {
		t.Fatalf("Zu genehmigen of the substitute: %v", list)
	}
	if r := call(t, sub.api, http.MethodGet, order, "", nil); r.code != 200 {
		t.Fatalf("the substitute reads: %d %s", r.code, r.raw)
	}
	decide(sub.api, order, aid, `{"decision": "reject"}`, 422)
	decide(sub.api, order, aid, `{"decision": "approve", "comment": "In Ordnung"}`, 200)
	a := read(admin, order)
	first := a.Stages[0].Steps[0]
	if a.Stage != 2 || a.Stages[0].State != "approved" || a.Stages[1].State != "open" || first.DecidedBy == nil || *first.DecidedBy != sub.person ||
		first.OnBehalf == nil || *first.OnBehalf != lead.person || first.Comment != "In Ordnung" {
		t.Fatalf("after stage 1: %+v", a)
	}

	// Stage 2: at least 2 of 3; the group refuses, two persons agree.
	decide(sec1.api, order, aid, `{"decision": "approve"}`, 200)
	decide(member.api, order, aid, `{"decision": "reject", "comment": "Kein Bedarf"}`, 200)
	if a := read(admin, order); a.State != "open" || a.Stages[1].State != "open" {
		t.Fatalf("one yes and one no of three, at least 2: still open, got %+v", a)
	}
	decide(member.api, order, aid, `{"decision": "approve"}`, 403)
	decide(sec2.api, order, aid, `{"decision": "approve"}`, 200)
	if a := read(admin, order); a.State != "approved" || a.Stages[1].State != "approved" {
		t.Fatalf("after the second yes: %+v", a)
	}
	if state, f := field(order); state != "approved" || f != "approved" {
		t.Fatalf("after the approval: state %s, approval %s", state, f)
	}
	decide(sec2.api, order, aid, `{"decision": "approve"}`, 409)
	if list := mine(sec2.api, "done"); len(list) != 1 || list[0]["state"] != "approved" {
		t.Fatalf("answered by sec2: %v", list)
	}
	if r := call(t, member.api, http.MethodGet, order, "", nil); r.code != 200 {
		t.Fatalf("the group that answered still reads: %d", r.code)
	}

	// A refusal closes the order with its reason as an entry of the timeline.
	second := newOrder("Zweiter Bildschirm")
	r := call(t, admin, http.MethodPost, second+"/approvals", `{"stages": [{"approvers": [`+person(lead.person)+`]}]}`, nil)
	if r.code != 201 {
		t.Fatalf("request 2: %d %s", r.code, r.raw)
	}
	decide(lead.api, second, r.body["id"].(string), `{"decision": "reject", "comment": "Zu teuer"}`, 200)
	if state, f := field(second); state != "rejected" || f != "rejected" {
		t.Fatalf("after the refusal: state %s, approval %s", state, f)
	}
	if bodies := entryBodies(timeline(t, admin, second)); len(bodies) != 1 || !strings.Contains(bodies[0], "Zu teuer") {
		t.Fatalf("the reason in the timeline: %v", bodies)
	}

	// Withdrawn: the questions are dropped, the asked no longer read.
	third := newOrder("Drucker")
	r = call(t, admin, http.MethodPost, third+"/approvals", `{"stages": [{"approvers": [`+person(sec1.person)+`]}]}`, nil)
	if r.code != 201 {
		t.Fatalf("request 3: %d %s", r.code, r.raw)
	}
	if w := call(t, sec1.api, http.MethodPost, third+"/approvals/"+r.body["id"].(string)+":withdraw", "", nil); w.code != 404 && w.code != 403 {
		t.Fatalf("an approver withdraws: %d %s", w.code, w.raw)
	}
	if w := call(t, admin, http.MethodPost, third+"/approvals/"+r.body["id"].(string)+":withdraw", "", nil); w.code != 200 || w.body["state"] != "withdrawn" {
		t.Fatalf("withdraw: %d %s", w.code, w.raw)
	}
	if _, f := field(third); f != "withdrawn" {
		t.Fatalf("field after withdrawing: %s", f)
	}
	if g := call(t, sec1.api, http.MethodGet, third, "", nil); g.code != 404 {
		t.Fatalf("the dropped approver reads: %d", g.code)
	}

	// Refused requests and types without approvals.
	fourth := newOrder("Ungültig")
	for _, body := range []string{
		`{"stages": []}`,
		`{"stages": [{"need": 2, "approvers": [` + person(lead.person) + `]}]}`,
		`{"stages": [{"approvers": [` + person("0190e0a0-0000-7000-8000-000000000000") + `]}]}`,
		`{"stages": [{"approvers": [` + person(lead.person) + `, ` + person(lead.person) + `]}]}`,
		`{"stages": [{"approvers": [{"type": "probe.probe", "id": "` + lead.person + `"}]}]}`,
	} {
		if r := call(t, admin, http.MethodPost, fourth+"/approvals", body, nil); r.code != 422 {
			t.Fatalf("request %s: %d %s", body, r.code, r.raw)
		}
	}
	if r := call(t, lead.api, http.MethodPost, fourth+"/approvals", `{"stages": [{"approvers": [`+person(lead.person)+`]}]}`, nil); r.code != 404 && r.code != 403 {
		t.Fatalf("asking without the right to change: %d %s", r.code, r.raw)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "Ohne"}`, nil)
	if p.code != 201 {
		t.Fatalf("probe: %d %s", p.code, p.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/records/probe.probe/"+p.body["id"].(string)+"/approvals", "", nil); r.code != 404 {
		t.Fatalf("a type without approvals: %d %s", r.code, r.raw)
	}

	// The history keeps every decision, the stream the events.
	h := call(t, admin, http.MethodGet, order+"/history", "", nil)
	if h.code != 200 || strings.Count(h.raw, `"action":"approval"`) != 5 || !strings.Contains(h.raw, `"on_behalf":"`+lead.person+`"`) {
		t.Fatalf("history: %d %s", h.code, h.raw)
	}
	ev := call(t, admin, http.MethodGet, "/api/v1/events?after=0&limit=1000", "", nil)
	for _, kind := range []string{`"approval_request"`, `"approval_decision"`, `"approval"`} {
		if !strings.Contains(ev.raw, kind) {
			t.Fatalf("events without %s: %s", kind, ev.raw)
		}
	}
}
