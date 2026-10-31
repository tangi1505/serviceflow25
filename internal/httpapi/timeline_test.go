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
	"strings"
	"testing"

	"github.com/google/uuid"

	"group24.de/itsm/internal/notify/smtptest"
)

// timelineUser is a person with an account and one role at root, signed in.
type timelineUser struct {
	api    *testAPI
	person string
}

// timelineRoles makes the roles of the timeline tests: a requester who reads only the title
// and may comment, an agent who also reads private entries, a reader who may only read.
func timelineRoles(t *testing.T, admin *testAPI, prefix string) {
	t.Helper()
	perms := map[string][]string{
		prefix + "_requester": {`{"type_name": "probe.probe", "action": "read", "scope": "all", "fields": {"read": ["title"]}}`,
			`{"type_name": "probe.probe", "action": "comment", "scope": "all"}`},
		prefix + "_agent": {`{"type_name": "probe.probe", "action": "read", "scope": "all"}`,
			`{"type_name": "probe.probe", "action": "comment", "scope": "all"}`,
			`{"type_name": "probe.probe", "action": "read_private", "scope": "all"}`},
		prefix + "_reader": {`{"type_name": "probe.probe", "action": "read", "scope": "all"}`},
	}
	for role, list := range perms {
		dropRole(t, admin, role)
		if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "`+role+`"}}`, nil); r.code != 201 {
			t.Fatalf("role %s: %d %s", role, r.code, r.raw)
		}
		for _, p := range list {
			if r := call(t, admin, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
				t.Fatalf("permission %s: %d %s", role, r.code, r.raw)
			}
		}
	}
}

// timelineLogin makes an account with a person (with the given fields) holding the role.
func timelineLogin(t *testing.T, admin *testAPI, name, role, person string) timelineUser {
	t.Helper()
	ctx := t.Context()
	acc, err := admin.auth.CreateAccount(ctx, name, name, testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", person, nil)
	if p.code != 201 {
		t.Fatalf("person: %d %s", p.code, p.raw)
	}
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "`+role+`", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	in := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "`+name+`", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if in.code != 200 {
		t.Fatalf("login %s: %d %s", name, in.code, in.raw)
	}
	return timelineUser{api: &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, in.hdr)}, person: pid.String()}
}

// timeline reads the timeline of a record as a list of items.
func timeline(t *testing.T, api *testAPI, base string) []map[string]any {
	t.Helper()
	r := call(t, api, http.MethodGet, base+"/timeline", "", nil)
	if r.code != 200 {
		t.Fatalf("timeline: %d %s", r.code, r.raw)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(r.raw), &items); err != nil {
		t.Fatal(err)
	}
	return items
}

// entryBodies are the texts of the entries in a timeline, in order.
func entryBodies(items []map[string]any) []string {
	var out []string
	for _, it := range items {
		if e, ok := it["entry"].(map[string]any); ok {
			out = append(out, e["body"].(string))
		}
	}
	return out
}

// The Prüfweg of PL42: a private entry appears in the timeline only for an account with
// read_private; a requester without update writes a public entry with comment, not a private
// one; a reader without comment writes none. Entries and changes come in order of time with
// the names of their authors; changes of fields a reader may not read are left out; the
// record's version stays; wrong input is refused; a type without the trait has no timeline;
// a purge takes the entries.
func TestTimelineOverHTTP(t *testing.T) {
	admin := apiServer(t)
	timelineRoles(t, admin, "pl42")
	requester := timelineLogin(t, admin, "apitest_pl42_req", "pl42_requester", `{"last_name": "PL42 Anforderer"}`)
	agent := timelineLogin(t, admin, "apitest_pl42_agent", "pl42_agent", `{"last_name": "PL42 Bearbeiter"}`)
	reader := timelineLogin(t, admin, "apitest_pl42_reader", "pl42_reader", `{"last_name": "PL42 Leser"}`)

	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "PL42 Drucker"}`, nil)
	if rec.code != 201 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)
	if r := call(t, admin, http.MethodPatch, base, `{"kind": "fault"}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("change kind: %d %s", r.code, r.raw)
	}
	add := func(u *testAPI, body string) resp {
		t.Helper()
		return call(t, u, http.MethodPost, base+"/timeline", body, nil)
	}
	if r := add(admin, `{"body": "Drucker steht im 3. Stock."}`); r.code != 201 || r.body["private"] != false || r.body["created_by"] == nil {
		t.Fatalf("public entry: %d %s", r.code, r.raw)
	}
	if r := add(admin, `{"body": "Intern: Ersatzteil bestellt.", "private": true}`); r.code != 201 || r.body["private"] != true {
		t.Fatalf("private entry: %d %s", r.code, r.raw)
	}
	for body, field := range map[string]string{`{"body": "  "}`: "body", `{"private": true}`: "body", `{"body": 7}`: "body",
		`{"body": "x", "private": "ja"}`: "private", `{"body": "x", "author": "y"}`: "author"} {
		if r := add(admin, body); r.code != 422 || !strings.Contains(r.raw, `"field":"`+field+`"`) {
			t.Fatalf("wrong entry %s: %d %s", body, r.code, r.raw)
		}
	}
	if r := add(admin, `{"body": "`+strings.Repeat("x", 100001)+`"}`); r.code != 422 || !strings.Contains(r.raw, "max_length") {
		t.Fatalf("too long: %d", r.code)
	}

	// The requester answers without update; a private entry or a change of the record is refused.
	if r := add(requester.api, `{"body": "Danke, ich warte."}`); r.code != 201 {
		t.Fatalf("requester's entry: %d %s", r.code, r.raw)
	}
	if r := add(requester.api, `{"body": "geheim", "private": true}`); r.code != 403 {
		t.Fatalf("requester's private entry: %d %s", r.code, r.raw)
	}
	if r := call(t, requester.api, http.MethodPatch, base, `{"title": "x"}`, map[string]string{"If-Match": `"2"`}); r.code != 403 {
		t.Fatalf("requester changes the record: %d %s", r.code, r.raw)
	}
	if r := add(reader.api, `{"body": "Ich lese nur."}`); r.code != 403 {
		t.Fatalf("reader's entry: %d %s", r.code, r.raw)
	}
	if r := add(agent.api, `{"body": "Intern: Techniker kommt morgen.", "private": true}`); r.code != 201 {
		t.Fatalf("agent's private entry: %d %s", r.code, r.raw)
	}

	// Who sees what.
	all := []string{"Drucker steht im 3. Stock.", "Intern: Ersatzteil bestellt.", "Danke, ich warte.", "Intern: Techniker kommt morgen."}
	public := []string{"Drucker steht im 3. Stock.", "Danke, ich warte."}
	for name, c := range map[string]struct {
		api  *testAPI
		want []string
	}{"admin": {admin, all}, "agent": {agent.api, all}, "requester": {requester.api, public}, "reader": {reader.api, public}} {
		if got := entryBodies(timeline(t, c.api, base)); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s sees %q, want %q", name, got, c.want)
		}
	}

	// Order, names and changes: first the create, then the change of kind, then the entries.
	items := timeline(t, admin, base)
	for i := 1; i < len(items); i++ {
		if items[i-1]["at"].(string) > items[i]["at"].(string) {
			t.Fatalf("not in order of time: %v", items)
		}
	}
	first := items[0]["change"].(map[string]any)
	if items[0]["kind"] != "change" || first["action"] != "create" || items[0]["actor_name"] != "API Test" {
		t.Fatalf("first item: %v", items[0])
	}
	var kindChange bool
	for _, it := range items {
		if c, ok := it["change"].(map[string]any); ok && c["field"] == "kind" && c["new"] == "fault" {
			kindChange = true
		}
		if e, ok := it["entry"].(map[string]any); ok && e["body"] == "Danke, ich warte." && it["actor_name"] != "apitest_pl42_req" {
			t.Fatalf("author of the requester's entry: %v", it)
		}
	}
	if !kindChange {
		t.Fatalf("the change of kind is missing: %v", items)
	}
	// The requester reads only the title: the change of kind is not in its timeline.
	for _, it := range timeline(t, requester.api, base) {
		if c, ok := it["change"].(map[string]any); ok && c["field"] == "kind" {
			t.Fatalf("requester sees a field it may not read: %v", it)
		}
	}
	// Nor in the history (found while building PL42: /history showed every field).
	if r := call(t, requester.api, http.MethodGet, base+"/history", "", nil); r.code != 200 ||
		strings.Contains(r.raw, `"field":"kind"`) || !strings.Contains(r.raw, `"field":"title"`) {
		t.Fatalf("requester's history: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, base+"/history", "", nil); !strings.Contains(r.raw, `"field":"kind"`) {
		t.Fatalf("admin's history: %s", r.raw)
	}

	// The record's version stays: the entries are no change of it.
	if r := call(t, admin, http.MethodGet, base, "", nil); r.hdr.Get("ETag") != `"2"` {
		t.Fatalf("version after entries: %s", r.hdr.Get("ETag"))
	}
	var events int
	if err := admin.pool.QueryRow(t.Context(), `SELECT count(*) FROM event WHERE kind = 'comment' AND record_id = $1 AND NOT payload ? 'body'`,
		rec.body["id"]).Scan(&events); err != nil || events != 4 {
		t.Fatalf("comment events: %d %v", events, err)
	}

	// A type without the trait has no timeline; the OpenAPI shows the path only for types with it.
	if r := call(t, admin, http.MethodGet, "/api/v1/records/core.person/"+requester.person+"/timeline", "", nil); r.code != 404 {
		t.Fatalf("timeline of core.person: %d %s", r.code, r.raw)
	}
	p := call(t, admin, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths, _ := p.body["paths"].(map[string]any)
	if paths["/api/v1/records/probe.probe/{id}/timeline"] == nil || paths["/api/v1/records/core.person/{id}/timeline"] != nil {
		t.Fatal("timeline paths in the OpenAPI are wrong")
	}

	// A purge takes the entries.
	call(t, admin, http.MethodDelete, base, "", nil)
	if r := call(t, admin, http.MethodPost, base+":purge", "", nil); r.code != 204 {
		t.Fatalf("purge: %d %s", r.code, r.raw)
	}
	var left int
	if err := admin.pool.QueryRow(t.Context(), `SELECT count(*) FROM entry WHERE record_id = $1`, rec.body["id"]).Scan(&left); err != nil || left != 0 {
		t.Fatalf("entries after the purge: %d %v", left, err)
	}
}

// A notification on comment sends the text of a public entry to everyone it names, that of a
// private entry only to persons who may read it (PL42 with WF15).
func TestTimelineNotifications(t *testing.T) {
	admin := apiServer(t)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	timelineRoles(t, admin, "pl42n")
	agent := timelineLogin(t, admin, "apitest_pl42n_agent", "pl42n_agent",
		`{"first_name": "Anna", "last_name": "PL42 `+suffix+`", "email": "anna-`+suffix+`@example.org"}`)
	requester := timelineLogin(t, admin, "apitest_pl42n_req", "pl42n_requester",
		`{"first_name": "Ben", "last_name": "PL42 `+suffix+`", "email": "ben-`+suffix+`@example.org"}`)

	title := "PL42 " + suffix
	n := call(t, admin, http.MethodPost, "/api/v1/records/core.notification", `{"name": "PL42 Eintrag `+suffix+`", "target_type": "probe.probe",
		"events": "comment", "condition": "record.title == \"`+title+`\"",
		"recipients": "core.person:`+agent.person+`, core.person:`+requester.person+`",
		"subject": "Neuer Eintrag: {{title}}", "body": "{{actor}} schreibt:\n{{entry}}"}`, nil)
	if n.code != 201 {
		t.Fatalf("notification: %d %s", n.code, n.raw)
	}
	defer func() {
		id := n.body["id"].(string)
		call(t, admin, http.MethodDelete, "/api/v1/records/core.notification/"+id, "", nil)
		call(t, admin, http.MethodPost, "/api/v1/records/core.notification/"+id+":purge", "", nil)
	}()
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "`+title+`"}`, nil)
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)

	if r := call(t, admin, http.MethodPost, base+"/timeline", `{"body": "Öffentlich `+suffix+`"}`, nil); r.code != 201 {
		t.Fatalf("public entry: %d %s", r.code, r.raw)
	}
	mails := waitForMail(t, admin, 2, func(m smtptest.Mail) bool { return strings.Contains(m.Body, "Öffentlich "+suffix) })
	for _, m := range mails {
		if m.Subject != "Neuer Eintrag: "+title || !strings.HasPrefix(m.Body, "API Test schreibt:\n") {
			t.Fatalf("mail of the public entry: %+v", m)
		}
	}
	if r := call(t, admin, http.MethodPost, base+"/timeline", `{"body": "Privat `+suffix+`", "private": true}`, nil); r.code != 201 {
		t.Fatalf("private entry: %d %s", r.code, r.raw)
	}
	mails = waitForMail(t, admin, 1, func(m smtptest.Mail) bool { return strings.Contains(m.Body, "Privat "+suffix) })
	if mails[0].To[0] != "anna-"+suffix+"@example.org" {
		t.Fatalf("the private entry went to %v", mails[0].To)
	}
}
