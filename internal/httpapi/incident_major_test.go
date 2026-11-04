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
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/notify/smtptest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
)

// The Prüfweg of SD5 (SD-Plan N3): declaring an incident a major incident takes a reason and
// the message to the users and mails the distribution list of the start data; three incidents
// that belong to it are resolved with it, with its solution, and each requester gets a mail;
// the portal shows the message under "Aktuell" to a person who cannot read the incident, until
// it is resolved.
func TestMajorIncidentOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd5"))
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
	ref := func(key string) string {
		t.Helper()
		var id string
		if err := api.pool.QueryRow(ctx, "SELECT ref FROM package_seed WHERE org_path = 'root' AND package = 'incident' AND key = $1", key).Scan(&id); err != nil {
			t.Fatalf("incident.%s: %v", key, err)
		}
		return id
	}
	// Declaring by itself comes switched off; the mail goes to the group of the start data.
	if r := call(t, api, http.MethodGet, "/api/v1/rules/incident_major_p1", "", nil); r.code != 200 || r.body["enabled"] != false {
		t.Fatalf("rule for P1: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/rules/incident_major_mail", "", nil); !strings.Contains(r.raw, "core.group:"+ref("verteiler")) {
		t.Fatalf("rule of the mail: %d %s", r.code, r.raw)
	}

	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	mara := create("core.person", `{"first_name": "Mara", "last_name": "Verteiler", "email": "mara@example.org", "language": "de"}`)
	create("core.group_member", `{"group": "`+ref("verteiler")+`", "person": "`+mara+`"}`)
	const inc = "/api/v1/records/incident.incident"
	newIncident := func(n int) string {
		t.Helper()
		person := create("core.person", fmt.Sprintf(`{"first_name": "Anna", "last_name": "SD5-%d", "email": "sd5-%d@example.org", "language": "de"}`, n, n))
		return create("incident.incident", fmt.Sprintf(`{"title": "SD5 Störung %d", "impact": "high", "urgency": "medium",
			"actors": {"requester": [{"type": "core.person", "id": %q}]}}`, n, person))
	}
	get := func(id string) resp { return call(t, api, http.MethodGet, inc+"/"+id, "", nil) }
	transition := func(id, body string) resp {
		v := get(id).body["version"]
		return call(t, api, http.MethodPost, inc+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)})
	}
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
				t.Fatalf("%s did not happen", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	mailTo := func(addr, text string) func() bool {
		return func() bool {
			return slices.ContainsFunc(api.smtp.Mail(), func(m smtptest.Mail) bool {
				return slices.Contains(m.To, addr) && strings.Contains(m.Subject+m.Body, text)
			})
		}
	}

	// Declaring takes a reason, and the message to the users with it.
	major := newIncident(0)
	if r := transition(major, `{"transition": "declare_major"}`); r.code != 422 || !strings.Contains(r.raw, "comment") {
		t.Fatalf("without a reason: %d %s", r.code, r.raw)
	}
	if r := transition(major, `{"transition": "declare_major", "comment": "Alle Dienststellen betroffen"}`); r.code != 422 || !strings.Contains(r.raw, "public_note") {
		t.Fatalf("without a message: %d %s", r.code, r.raw)
	}
	const note = "Die Vorgangsbearbeitung ist nicht erreichbar. Wir arbeiten daran."
	if r := transition(major, `{"transition": "declare_major", "comment": "Alle Dienststellen betroffen", "fields": {"public_note": "`+note+`"}}`); r.code != 200 {
		t.Fatalf("declare: %d %s", r.code, r.raw)
	}
	if b := get(major).body; b["major"] != true || b["state"] != "in_progress" {
		t.Fatalf("after declaring: %v", b)
	}
	waitFor("the mail to the distribution list", mailTo("mara@example.org", "Major Incident"))

	// The portal shows the message to a person who cannot read the incident, and nothing more.
	timelineRoles(t, api, "sd5")
	user := timelineLogin(t, api, "apitest_sd5", "sd5_reader", `{"last_name": "SD5"}`)
	if r := call(t, user.api, http.MethodGet, inc+"/"+major, "", nil); r.code == 200 {
		t.Fatalf("the person reads the incident: %s", r.raw)
	}
	notices := func() []map[string]any {
		t.Helper()
		r := call(t, user.api, http.MethodGet, "/api/v1/portal/notices", "", nil)
		var list []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("notices: %d %s", r.code, r.raw)
		}
		if strings.Contains(r.raw, "SD5 Störung") {
			t.Fatalf("the notice shows the title: %s", r.raw)
		}
		return list
	}
	if list := notices(); len(list) != 1 || list[0]["id"] != major || list[0]["text"] != note || list[0]["state"] != "in_progress" ||
		list[0]["state_label"].(map[string]any)["de"] != "In Arbeit" || list[0]["notice"] != ref("aktuell") {
		t.Fatalf("notices: %v", list)
	}

	// Three incidents belong to it; resolving it resolves them with its solution.
	parts := []string{newIncident(1), newIncident(2), newIncident(3)}
	for _, p := range parts {
		r := call(t, api, http.MethodPost, "/api/v1/relations",
			fmt.Sprintf(`{"kind": "incident.part_of", "from": {"type": "incident.incident", "id": %q}, "to": {"type": "incident.incident", "id": %q}}`, p, major), nil)
		if r.code != 201 {
			t.Fatalf("part of: %d %s", r.code, r.raw)
		}
	}
	if r := transition(parts[2], `{"transition": "start"}`); r.code != 200 {
		t.Fatalf("start: %d %s", r.code, r.raw)
	}
	fixed := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.solution_kind?limit=50", "", nil).body["items"].([]any) {
		if k := it.(map[string]any); k["key"] == "fixed" {
			fixed = k["id"].(string)
		}
	}
	if r := transition(major, `{"transition": "resolve", "fields": {"solution": "Datenbankserver neu gestartet", "solution_kind": "`+fixed+`"}}`); r.code != 200 {
		t.Fatalf("resolve: %d %s", r.code, r.raw)
	}
	waitFor("the incidents resolved with it", func() bool {
		for _, p := range parts {
			if get(p).body["state"] != "resolved" {
				return false
			}
		}
		return true
	})
	for i, p := range parts {
		b := get(p).body
		if b["solution"] != "Datenbankserver neu gestartet" || fmt.Sprint(b["solution_kind"]) != fixed {
			t.Fatalf("part %d: %v", i+1, b)
		}
		// The subject carries the number in front since SD10.
		waitFor(fmt.Sprintf("the mail to requester %d", i+1), mailTo(fmt.Sprintf("sd5-%d@example.org", i+1), fmt.Sprintf("[INC-%v] Ihre Störung ist gelöst", b["number"])))
	}
	if list := notices(); len(list) != 0 {
		t.Fatalf("notices after the solution: %v", list)
	}
}
