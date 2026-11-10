// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/notify/smtptest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD10 (Inventar G163, KF14): the mails of the start data of incident carry the
// running number in front of the subject. Anna reports in her name and gets the receipt; her
// answer goes to the group while no one works on it, later to the assignee; an answer of the
// service desk goes to her, an internal note does not; resolved, she gets the solution as plain
// text and the request to confirm it; closed, she hears of it.
func TestIncidentMailsOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd10"))
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
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	// Gerd is in the group, Ben will work on it; Anna reports and answers with an account.
	gerd := create("core.person", `{"first_name": "Gerd", "last_name": "SD10", "email": "gerd10@example.org", "language": "de"}`)
	ben := create("core.person", `{"first_name": "Ben", "last_name": "SD10", "email": "ben10@example.org", "language": "de"}`)
	group := create("core.group", `{"name": "SD10 Erste Ebene"}`)
	create("core.group_member", `{"group": "`+group+`", "person": "`+gerd+`"}`)
	role := "sd10_requester"
	dropRole(t, api, role)
	if r := call(t, api, http.MethodPost, "/api/v1/roles", `{"name": "`+role+`", "label": {"de": "SD10"}}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, p := range []string{`{"type_name": "incident.incident", "action": "create", "scope": "all"}`,
		`{"type_name": "incident.incident", "action": "read", "scope": "involved"}`,
		`{"type_name": "incident.incident", "action": "comment", "scope": "involved"}`} {
		if r := call(t, api, http.MethodPut, "/api/v1/roles/"+role+"/permissions", p, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	anna := timelineLogin(t, api, "apitest_sd10", role, `{"first_name": "Anna", "last_name": "SD10", "email": "anna10@example.org", "language": "de"}`)

	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !ok() {
			for _, d := range api.rt.Databases() {
				if err := api.ruleEvents.Dispatch(d.Bind(ctx)); err != nil {
					t.Fatal(err)
				}
				if err := api.notifyEvents.Dispatch(d.Bind(ctx)); err != nil {
					t.Fatal(err)
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen: %v", what, api.smtp.Mail())
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	mailTo := func(addr, subject string) (smtptest.Mail, bool) {
		for _, m := range api.smtp.Mail() {
			if slices.Contains(m.To, addr) && strings.HasPrefix(m.Subject, subject) {
				return m, true
			}
		}
		return smtptest.Mail{}, false
	}
	expect := func(addr, subject string) smtptest.Mail {
		t.Helper()
		waitFor("the mail "+subject+" to "+addr, func() bool { _, ok := mailTo(addr, subject); return ok })
		m, _ := mailTo(addr, subject)
		return m
	}

	// A reference in a mail shows its title, not its id (found in SD8).
	network := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.category?limit=100", "", nil).body["items"].([]any) {
		if c := it.(map[string]any); c["key"] == "network" {
			network = c["id"].(string)
		}
	}
	create("core.notification", `{"name": "SD10 Kategorie", "target_type": "incident.incident", "events": "create",
		"recipients": "actors.requester", "notify_actor": true, "subject": "SD10 Kategorie {{record.category}}", "body": "x"}`)

	// Anna reports: the receipt with the number in front.
	r := call(t, anna.api, http.MethodPost, "/api/v1/records/incident.incident", `{"title": "SD10 Drucker druckt leer", "impact": "low", "urgency": "low",
		"category": "`+network+`", "actors": {"requester": [{"type": "core.person", "id": "`+anna.person+`"}]}}`, nil)
	if r.code != 201 {
		t.Fatalf("Anna reports: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	const inc = "/api/v1/records/incident.incident"
	// The API gives the running number as a number, the mail with its prefix (PL49).
	number := fmt.Sprintf("INC-%v", call(t, api, http.MethodGet, inc+"/"+id, "", nil).body["number"])
	tag := "[" + number + "] "
	if m := expect("anna10@example.org", tag+"Ihre Störung ist eingegangen: SD10 Drucker druckt leer"); !strings.Contains(m.Body, number) {
		t.Fatalf("receipt: %+v", m)
	}
	expect("anna10@example.org", "SD10 Kategorie Netz und Telefonie")
	patch := func(body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, inc+"/"+id, "", nil).body["version"]
		if r := call(t, api, http.MethodPatch, inc+"/"+id, body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("patch %s: %d %s", body, r.code, r.raw)
		}
	}
	patch(`{"actors": {"group": [{"type": "core.group", "id": "` + group + `"}]}}`)
	write := func(who *testAPI, body string) {
		t.Helper()
		if r := call(t, who, http.MethodPost, inc+"/"+id+"/timeline", body, nil); r.code != 201 {
			t.Fatalf("entry %s: %d %s", body, r.code, r.raw)
		}
	}

	// Her answer without an assignee goes to the group.
	write(anna.api, `{"body": "Es ist der Drucker im Flur."}`)
	if m := expect("gerd10@example.org", tag+"Antwort des Anforderers"); !strings.Contains(m.Body, "Es ist der Drucker im Flur.") {
		t.Fatalf("answer to the group: %+v", m)
	}
	// The service desk answers her; an internal note does not reach her.
	patch(`{"actors": {"assignee": [{"type": "core.person", "id": "` + ben + `"}]}}`)
	write(api, `{"body": "Ist ein Ausdruck festgestellt?", "private": true}`)
	write(api, `{"body": "Bitte starten Sie den Drucker neu."}`)
	if m := expect("anna10@example.org", tag+"Neue Antwort zu Ihrer Störung"); !strings.Contains(m.Body, "Bitte starten Sie den Drucker neu.") {
		t.Fatalf("answer to Anna: %+v", m)
	}
	// Now the assignee hears of her answer, the group no more.
	write(anna.api, `{"body": "Hat nicht geholfen."}`)
	expect("ben10@example.org", tag+"Antwort des Anforderers")
	for _, m := range api.smtp.Mail() {
		switch {
		case strings.Contains(m.Body, "Ist ein Ausdruck festgestellt?"):
			t.Fatalf("the internal note went out: %+v", m)
		case slices.Contains(m.To, "gerd10@example.org") && strings.Contains(m.Body, "Hat nicht geholfen."):
			t.Fatalf("the group heard of the answer with an assignee: %+v", m)
		case slices.Contains(m.To, "anna10@example.org") && strings.Contains(m.Subject, "Antwort des Anforderers"):
			t.Fatalf("Anna heard of her own answer: %+v", m)
		}
	}

	// Resolved: the solution as plain text and the request to confirm; closed: one more mail.
	fixed := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.solution_kind?limit=50", "", nil).body["items"].([]any) {
		if k := it.(map[string]any); k["key"] == "fixed" {
			fixed = k["id"].(string)
		}
	}
	move := func(body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, inc+"/"+id, "", nil).body["version"]
		if r := call(t, api, http.MethodPost, inc+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
	}
	move(`{"transition": "resolve", "fields": {"solution": "<p>Toner <b>getauscht</b></p>", "solution_kind": "` + fixed + `"}}`)
	if m := expect("anna10@example.org", tag+"Ihre Störung ist gelöst"); !strings.Contains(m.Body, "Toner getauscht") ||
		strings.Contains(m.Body, "<b>") || !strings.Contains(m.Body, "bestätigen") {
		t.Fatalf("solved: %+v", m)
	}
	move(`{"transition": "close"}`)
	expect("anna10@example.org", tag+"Ihre Störung ist geschlossen")
}
