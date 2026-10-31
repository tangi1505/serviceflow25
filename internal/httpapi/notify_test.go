// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/notify/smtptest"
)

// The Prüfweg of WF15: a new record sends a mail to each member of its assigned group, in
// German or English as each person has set, with the placeholders filled in; the person who
// created it gets none, a person without an address none, a person named twice one. A rule's
// notify goes out as mail with its message. Wrong notifications are refused.
func TestNotificationsOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	title := "WF15 " + suffix
	var made []string
	defer func() {
		call(t, api, http.MethodDelete, "/api/v1/rules/wf15_check", "", nil)
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}()
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		id := r.body["id"].(string)
		made = append(made, typeName+"/"+id)
		return id
	}
	anna := create("core.person", `{"first_name": "Anna", "last_name": "WF15 `+suffix+`", "email": "anna-`+suffix+`@example.org", "language": "de"}`)
	ben := create("core.person", `{"first_name": "Ben", "last_name": "WF15 `+suffix+`", "email": "ben-`+suffix+`@example.org", "language": "en"}`)
	nomail := create("core.person", `{"first_name": "Ohne", "last_name": "WF15 `+suffix+`"}`)
	group := create("core.group", `{"name": "WF15 Netz `+suffix+`"}`)
	for _, p := range []string{anna, ben, nomail, api.person.String()} {
		create("core.group_member", `{"group": "`+group+`", "person": "`+p+`"}`)
	}

	// Wrong notifications: an unknown event, a recipient that is none, a placeholder of no field.
	for _, c := range []struct{ body, field string }{
		{`"events": "fly", "recipients": "actors.assigned", "subject": "x", "body": "x"`, "events"},
		{`"events": "create", "recipients": "jemand", "subject": "x", "body": "x"`, "recipients"},
		{`"events": "create", "recipients": "actors.nobody", "subject": "x", "body": "x"`, "recipients"},
		{`"events": "create", "recipients": "actors.assigned", "subject": "{{record.nofield}}", "body": "x"`, "subject"},
		{`"events": "create", "recipients": "actors.assigned", "subject": "x", "body": "{{whatever}}"`, "body"},
	} {
		r := call(t, api, http.MethodPost, "/api/v1/records/core.notification", `{"name": "WF15 falsch `+suffix+`", "target_type": "probe.probe", `+c.body+`}`, nil)
		if r.code != 422 || !strings.Contains(r.raw, `"`+c.field+`"`) {
			t.Fatalf("wrong notification %s: %d %s", c.body, r.code, r.raw)
		}
	}
	// To the assigned group and, named twice, to Anna; German in the column, English as a
	// translation.
	n := create("core.notification", `{"name": "WF15 Neu `+suffix+`", "target_type": "probe.probe", "events": "create",
		"recipients": "actors.assigned, core.person:`+anna+`",
		"subject": "Neuer Vorgang: {{record.title}}", "body": "{{type}} „{{record.title}}“ (Art {{record.kind}}) von {{actor}}.\n{{link}}"}`)
	if r := call(t, api, http.MethodPatch, "/api/v1/records/core.notification/"+n,
		`{"subject": "New record: {{record.title}}", "body": "{{type}} \"{{record.title}}\" (kind {{record.kind}}) by {{actor}}.\n{{link}}"}`,
		map[string]string{"If-Match": `"1"`, "Content-Language": "en"}); r.code != 200 {
		t.Fatalf("English: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/records/core.notification/"+n, "", map[string]string{"Accept-Language": "en"}); r.body["subject"] != "New record: {{record.title}}" {
		t.Fatalf("English is not kept: %s", r.raw)
	}

	id := create("probe.probe", `{"title": "`+title+` Router", "kind": "fault", "actors": {"assigned": [{"type": "core.group", "id": "`+group+`"}]}}`)
	mails := waitForMail(t, api, 2, func(m smtptest.Mail) bool { return strings.Contains(m.Subject, title) })
	byTo := map[string]smtptest.Mail{}
	for _, m := range mails {
		byTo[m.To[0]] = m
	}
	de, en := byTo["anna-"+suffix+"@example.org"], byTo["ben-"+suffix+"@example.org"]
	link := "http://itsm.test/types/probe.probe/" + id
	if de.Subject != "Neuer Vorgang: "+title+" Router" || !strings.Contains(de.Body, "„"+title+" Router“ (Art Störung) von ") ||
		!strings.Contains(de.Body, link) || de.From != "itsm@test.local" {
		t.Fatalf("German mail: %+v", de)
	}
	if en.Subject != "New record: "+title+" Router" || !strings.Contains(en.Body, `"`+title+` Router" (kind Fault) by `) || !strings.Contains(en.Body, link) {
		t.Fatalf("English mail: %+v", en)
	}

	// A rule's notify goes out as mail with its message, to the person it names.
	if r := call(t, api, http.MethodPut, "/api/v1/rules/wf15_check", `{"type_name": "probe.probe", "trigger": "create", "mode": "after",
		"condition": "record.title.startsWith(\"`+title+` Regel\")", "actions": [{"notify": {"to": ["core.person:`+ben+`"], "message": "Bitte prüfen"}}]}`, nil); r.code != 201 {
		t.Fatalf("rule: %d %s", r.code, r.raw)
	}
	create("probe.probe", `{"title": "`+title+` Regel"}`)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if err := api.ruleEvents.Dispatch(ctx); err != nil {
			t.Fatal(err)
		}
		if err := api.notifyEvents.Dispatch(ctx); err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(api.smtp.Mail(), func(m smtptest.Mail) bool {
			return m.To[0] == "ben-"+suffix+"@example.org" && strings.Contains(m.Body, "Bitte prüfen")
		})
		if i >= 0 {
			if m := api.smtp.Mail()[i]; !strings.Contains(m.Subject, title+" Regel") {
				t.Fatalf("mail of the rule: %+v", m)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no mail of the rule: %+v", api.smtp.Mail())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForMail dispatches until want mails that match have arrived, then a moment longer to
// see that no more come; it fails on more or fewer.
func waitForMail(t *testing.T, api *testAPI, want int, match func(smtptest.Mail) bool) []smtptest.Mail {
	t.Helper()
	ctx := t.Context()
	got := func() []smtptest.Mail {
		var out []smtptest.Mail
		for _, m := range api.smtp.Mail() {
			if match(m) {
				out = append(out, m)
			}
		}
		return out
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(got()) < want {
		// The dispatcher of every database (PL47c), as the worker runs them.
		for _, d := range api.rt.Databases() {
			if err := api.notifyEvents.Dispatch(d.Bind(ctx)); err != nil {
				t.Fatal(err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d mails: %+v", len(got()), want, api.smtp.Mail())
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	if mails := got(); len(mails) != want {
		t.Fatalf("%d mails, want %d: %+v", len(mails), want, mails)
	}
	return got()
}
