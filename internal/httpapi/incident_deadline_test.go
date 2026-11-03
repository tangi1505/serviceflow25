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

	"github.com/google/uuid"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/notify/smtptest"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/seed"
	"group24.de/itsm/internal/sla"
	"group24.de/itsm/internal/tenant"
)

// The Prüfweg of SD3 (SD-Plan E6): the packages sd and incident bring their start data, once
// (N9); an incident's deadlines count in the service time, holidays included, P1 round the
// clock; at 80 % the responsible person of the assigned group gets a mail (N10); a wait holds
// the clock of the solution; five days of the service time after "resolved" it is closed. A new
// tenant gets the start data as well.
func TestIncidentDeadlinesOverHTTP(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_sd3"))
	api := apiServer(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)
	api.sla.Now = func() time.Time { return now }

	// The start data: 27 entries of sd (calendar, hours, holidays, reasons to wait), 18 of
	// incident (nine deadlines, nine rules); a second install creates nothing, nor does it
	// bring back what the customer deleted.
	recs := records.New(api.rt)
	starter := seed.New(api.rt, recs, api.rules)
	var pkgs []meta.Package
	for _, c := range []struct {
		dir  string
		want int
	}{{"../../packages/sd", 27}, {"../../packages/incident", 18}} {
		p := installPackage(t, ctx, api.rt, c.dir)
		if n, err := starter.Package(ctx, p); err != nil || n != c.want {
			t.Fatalf("start data of %s: %d %v, want %d", p.Name, n, err, c.want)
		}
		pkgs = append(pkgs, p)
	}
	api.sla.Wrote(ctx, sla.TypeSLA)
	ref := func(pkg, key string) string {
		t.Helper()
		var id string
		if err := api.pool.QueryRow(ctx, "SELECT ref FROM package_seed WHERE org_path = 'root' AND package = $1 AND key = $2", pkg, key).Scan(&id); err != nil {
			t.Fatalf("%s.%s: %v", pkg, key, err)
		}
		return id
	}
	third := ref("sd", "warten_dritte")
	if r := call(t, api, http.MethodDelete, "/api/v1/records/core.wait_reason/"+third, "", nil); r.code != 204 {
		t.Fatalf("delete a reason to wait: %d %s", r.code, r.raw)
	}
	for _, p := range pkgs {
		if n, err := starter.Package(ctx, p); err != nil || n != 0 {
			t.Fatalf("start data again for %s: %d %v", p.Name, n, err)
		}
	}
	if r := call(t, api, http.MethodGet, "/api/v1/records/core.wait_reason?limit=10", "", nil); len(r.body["items"].([]any)) != 2 {
		t.Fatalf("reasons to wait after the second install: %s", r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/rules/incident_close_resolved", "", nil); r.code != 200 || r.body["enabled"] != true {
		t.Fatalf("rule of the start data: %d %s", r.code, r.raw)
	}

	// Only a role that groups hold has responsible persons.
	if r := call(t, api, http.MethodPut, "/api/v1/rules/sd3_wrong", `{"type_name": "incident.incident", "trigger": "create", "mode": "after",
		"actions": [{"notify": {"to": ["actors.requester.responsible"], "message": "x"}}]}`, nil); r.code != 422 || !strings.Contains(r.raw, "no role of groups") {
		t.Fatalf("lead of the requester: %d %s", r.code, r.raw)
	}

	// The team: a group whose lead is Lena; the incidents go to it.
	create := func(typeName, body string) string {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		return r.body["id"].(string)
	}
	lena := create("core.person", `{"first_name": "Lena", "last_name": "Leitung", "email": "lena@example.org", "language": "de"}`)
	group := create("core.group", `{"name": "SD3 Erste Ebene", "responsible": "`+lena+`"}`)
	const inc = "/api/v1/records/incident.incident"
	// A notification may name the lead too: Lena hears of every new incident of her group.
	note := `{"name": "SD3 neu", "target_type": "incident.incident", "events": "create", "subject": "SD3 neu {{record.title}}", "body": "Neue Störung", "recipients": `
	if r := call(t, api, http.MethodPost, "/api/v1/records/core.notification", note+`"actors.requester.responsible"}`, nil); r.code != 422 {
		t.Fatalf("notification to the lead of the requester: %d %s", r.code, r.raw)
	}
	create("core.notification", note+`"actors.group.responsible"}`)
	newIncident := func(title, impact, urgency string) string {
		t.Helper()
		return create("incident.incident", `{"title": "`+title+`", "impact": "`+impact+`", "urgency": "`+urgency+`",
			"actors": {"group": [{"type": "core.group", "id": "`+group+`"}]}}`)
	}
	get := func(id string) resp { return call(t, api, http.MethodGet, inc+"/"+id, "", nil) }
	transition := func(id, body string) {
		t.Helper()
		v := get(id).body["version"]
		if r := call(t, api, http.MethodPost, inc+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
	}
	type clockRow struct {
		id    uuid.UUID
		gen   int64
		state string
		due   time.Time
	}
	clockOf := func(id, key string) clockRow {
		t.Helper()
		var c clockRow
		if err := api.pool.QueryRow(ctx, "SELECT id, generation, state, COALESCE(due_at, 'epoch') FROM clock WHERE record_id = $1 AND sla = $2", id, ref("incident", key)).
			Scan(&c.id, &c.gen, &c.state, &c.due); err != nil {
			t.Fatalf("clock %s: %v", key, err)
		}
		return c
	}
	due := func(id, key string, want time.Time) {
		t.Helper()
		if c := clockOf(id, key); !c.due.Equal(want) {
			t.Fatalf("%s due at %v, want %v", key, c.due.In(want.Location()), want)
		}
	}
	run := func(c clockRow) {
		t.Helper()
		if _, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen}); err != nil {
			t.Fatal(err)
		}
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
	berlin, _ := time.LoadLocation("Europe/Berlin")
	at := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2026, month, day, hour, minute, 0, 0, berlin)
	}

	// P2 on Friday 6 November at 17:00: the reaction is due at 17:30, the solution after one hour
	// on Friday and seven on Monday, at 14:00; the list shows the next deadline.
	now = at(time.November, 6, 17, 0)
	p2 := newIncident("SD3 Drucker", "medium", "high")
	due(p2, "sla_p2_reaction", at(time.November, 6, 17, 30))
	waitFor("the mail of the notification", func() bool {
		return slices.ContainsFunc(api.smtp.Mail(), func(m smtptest.Mail) bool {
			return m.Subject == "SD3 neu SD3 Drucker" && m.To[0] == "lena@example.org"
		})
	})
	due(p2, "sla_p2_solution", at(time.November, 9, 14, 0))
	if d, _ := time.Parse(time.RFC3339, fmt.Sprint(get(p2).body["sla_due_at"])); !d.Equal(at(time.November, 6, 17, 30)) {
		t.Fatalf("deadline of the record: %v", get(p2).body["sla_due_at"])
	}

	// At 80 % of the reaction, 17:24, Lena gets a mail; no one else does.
	now = at(time.November, 6, 17, 25)
	run(clockOf(p2, "sla_p2_reaction"))
	lead := func(m smtptest.Mail) bool { return strings.Contains(m.Body, "80 % der Reaktionsfrist (P2)") }
	waitFor("the mail to the lead", func() bool { return slices.ContainsFunc(api.smtp.Mail(), lead) })
	for _, m := range api.smtp.Mail() {
		if lead(m) && (len(m.To) != 1 || m.To[0] != "lena@example.org" || !strings.Contains(m.Subject, "SD3 Drucker")) {
			t.Fatalf("mail of the escalation: %+v", m)
		}
	}

	// Started at 17:28 the reaction is met. Waiting on Monday from 9 to 11 for the requester
	// holds the solution: due two hours later, at 16:00.
	now = at(time.November, 6, 17, 28)
	transition(p2, `{"transition": "start"}`)
	if c := clockOf(p2, "sla_p2_reaction"); c.state != sla.StateStopped {
		t.Fatalf("reaction after the start: %+v", c)
	}
	now = at(time.November, 9, 9, 0)
	transition(p2, `{"transition": "wait", "fields": {"wait_reason": "`+ref("sd", "warten_rueckmeldung")+`"}}`)
	if c := clockOf(p2, "sla_p2_solution"); c.state != sla.StatePaused {
		t.Fatalf("solution while waiting: %+v", c)
	}
	now = at(time.November, 9, 11, 0)
	transition(p2, `{"transition": "resume"}`)
	due(p2, "sla_p2_solution", at(time.November, 9, 16, 0))

	// Resolved on Monday at 12:00: closed five days of the service time later, on Monday the
	// 16th at 12:00.
	now = at(time.November, 9, 12, 0)
	fixed := ""
	for _, it := range call(t, api, http.MethodGet, "/api/v1/records/sd.solution_kind?limit=50", "", nil).body["items"].([]any) {
		if k := it.(map[string]any); k["key"] == "fixed" {
			fixed = k["id"].(string)
		}
	}
	transition(p2, `{"transition": "resolve", "fields": {"solution": "Treiber neu", "solution_kind": "`+fixed+`"}}`)
	due(p2, "sla_close", at(time.November, 16, 12, 0))
	now = at(time.November, 16, 12, 1)
	run(clockOf(p2, "sla_close"))
	waitFor("the automatic close", func() bool { return get(p2).body["state"] == "closed" })

	// P1 counts round the clock: Friday 17:00 gives 21:00. P3 on Christmas Eve at 17:00: one
	// hour on Thursday, the two holidays and the weekend free, three hours on Monday.
	now = at(time.November, 6, 17, 0)
	p1 := newIncident("SD3 Netz weg", "high", "high")
	due(p1, "sla_p1_solution", at(time.November, 6, 21, 0))
	now = at(time.December, 24, 17, 0)
	p3 := newIncident("SD3 Maus", "low", "high")
	due(p3, "sla_p3_reaction", at(time.December, 28, 10, 0))

	// A new tenant gets its own copy of the start data, rules with its name.
	res, err := tenant.Create(ctx, api.pool, tenant.Options{Name: "sd3_nord"})
	if err != nil || res.Started != 45 {
		t.Fatalf("new tenant: %+v %v", res, err)
	}
	var owner string
	if err := api.pool.QueryRow(ctx, "SELECT tenant::text FROM rule WHERE name = 'incident_close_resolved_sd3_nord'").Scan(&owner); err != nil || owner != "root.sd3_nord" {
		t.Fatalf("rule of the tenant: %q %v", owner, err)
	}
}
