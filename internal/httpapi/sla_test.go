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
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/sla"
)

// The Prüfweg of WF9: a clock starts when the record is created, pauses while it waits, and
// the remaining time is right after it goes on; at the deadline the job marks the breach and
// the history shows it; the solution stops the clock. An SLA that comes to hold later (a
// higher priority) counts from the creation of the record; in the trash the clocks hold. A
// calendar moves the deadline over the weekend. The test sets the time of the clocks.
func TestSLAOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	now := time.Now().UTC().Truncate(time.Second)
	api.sla.Now = func() time.Time { return now }
	const base = "/api/v1/records/probe.probe"
	var made []string // type/id to remove at the end, in order
	create := func(typeName, body string) map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		made = append(made, typeName+"/"+r.body["id"].(string))
		return r.body
	}
	defer func() {
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}()
	newSLA := func(name, applies string, minutes int, extra string) {
		t.Helper()
		create("core.sla", fmt.Sprintf(`{"name": "WF9 %s %s", "target_type": "probe.probe", "target_minutes": %d,
			"applies": %q, "pause": "record.state == \"waiting\"", "stop": "record.state in [\"resolved\", \"closed\"]"%s}`,
			name, suffix, minutes, applies, extra))
	}
	type clockView struct {
		ID         string     `json:"id"`
		SLAName    string     `json:"sla_name"`
		State      string     `json:"state"`
		Breached   bool       `json:"breached"`
		Elapsed    float64    `json:"elapsed_minutes"`
		Remaining  float64    `json:"remaining_minutes"`
		Calendar   *string    `json:"calendar"`
		StartedAt  time.Time  `json:"started_at"`
		DueAt      *time.Time `json:"due_at"`
		BreachedAt *time.Time `json:"breached_at"`
		StoppedAt  *time.Time `json:"stopped_at"`
	}
	clocks := func(id string) map[string]clockView {
		t.Helper()
		r := call(t, api, http.MethodGet, base+"/"+id+"/clocks", "", nil)
		var list []clockView
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &list) != nil {
			t.Fatalf("clocks: %d %s", r.code, r.raw)
		}
		out := map[string]clockView{}
		for _, c := range list {
			out[strings.Fields(c.SLAName)[1]] = c
		}
		return out
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 0.2 }
	transition := func(id, body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, base+"/"+id, "", nil).body["version"]
		r := call(t, api, http.MethodPost, base+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)})
		if r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
	}

	// An SLA with a condition that does not compile, or for a type that is not there: 422.
	r := call(t, api, http.MethodPost, "/api/v1/records/core.sla", `{"name": "WF9 bad `+suffix+`", "target_type": "probe.probe",
		"target_minutes": 10, "stop": "record.state =="}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, `"stop"`) {
		t.Fatalf("condition that does not compile: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPost, "/api/v1/records/core.sla", `{"name": "WF9 bad `+suffix+`", "target_type": "probe.nothing",
		"target_minutes": 10, "stop": "true"}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, "target_type") {
		t.Fatalf("unknown type: %d %s", r.code, r.raw)
	}

	// A: 60 minutes round the clock. Created at T0, waiting from T0+20 to T0+50: at T0+60 there
	// are 30 minutes left, the deadline is T0+90.
	newSLA("A", `record.title.startsWith("WF9 A")`, 60, "")
	a := create("probe.probe", `{"title": "WF9 A `+suffix+`"}`)["id"].(string)
	t0 := now
	c := clocks(a)["A"]
	if c.State != sla.StateRunning || c.Remaining != 60 || c.DueAt == nil || !c.DueAt.Equal(t0.Add(time.Hour)) || c.Calendar != nil {
		t.Fatalf("clock of a new record: %+v", c)
	}
	now = t0.Add(10 * time.Minute)
	transition(a, `{"transition": "start"}`)
	now = t0.Add(20 * time.Minute)
	transition(a, `{"transition": "wait"}`)
	now = t0.Add(50 * time.Minute)
	if c = clocks(a)["A"]; c.State != sla.StatePaused || c.Elapsed != 20 || c.Remaining != 40 || c.DueAt != nil {
		t.Fatalf("waiting: %+v", c)
	}
	transition(a, `{"transition": "resume"}`)
	now = t0.Add(60 * time.Minute)
	if c = clocks(a)["A"]; c.State != sla.StateRunning || c.Elapsed != 30 || c.Remaining != 30 || !c.DueAt.Equal(t0.Add(90*time.Minute)) {
		t.Fatalf("after the pause: %+v", c)
	}
	// The job of the run before the pause does nothing; the current one marks the breach at
	// the deadline, and the history shows it.
	var gen int64
	if err := api.pool.QueryRow(ctx, "SELECT generation FROM clock WHERE id = $1", c.ID).Scan(&gen); err != nil {
		t.Fatal(err)
	}
	clockID := uuid.MustParse(c.ID)
	now = t0.Add(91 * time.Minute)
	if breached, err := api.sla.Due(ctx, sla.DueArgs{Clock: clockID, Generation: gen - 2}); err != nil || breached {
		t.Fatalf("job of an older generation: %v %v", breached, err)
	}
	var jobs int
	if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE queue = $1 AND args->>'clock' = $2", slaTestQueue, c.ID).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatalf("one job for each run, two runs: %d %v", jobs, err)
	}
	if breached, err := api.sla.Due(ctx, sla.DueArgs{Clock: clockID, Generation: gen}); err != nil || !breached {
		t.Fatalf("job at the deadline: %v %v", breached, err)
	}
	if c = clocks(a)["A"]; !c.Breached || !c.BreachedAt.Equal(t0.Add(90*time.Minute)) || c.Remaining != -1 {
		t.Fatalf("breached: %+v", c)
	}
	h := call(t, api, http.MethodGet, base+"/"+a+"/history", "", nil)
	if !strings.Contains(h.raw, `"breach"`) || !strings.Contains(h.raw, "WF9 A "+suffix) {
		t.Fatalf("history without the breach: %s", h.raw)
	}
	// The solution at T0+95 stops the clock after 65 minutes of work; it stays breached.
	now = t0.Add(95 * time.Minute)
	transition(a, `{"transition": "resolve", "fields": {"solution": "Neu gestartet"}}`)
	if c = clocks(a)["A"]; c.State != sla.StateStopped || !c.Breached || c.Elapsed != 65 || c.StoppedAt == nil {
		t.Fatalf("resolved: %+v", c)
	}

	// B: priority 3 has 480 minutes, priority 1 has 60. Raised to 1 after 90 minutes, the
	// clock of priority 1 counts from the creation and is past its target at once; the one of
	// priority 3 is cancelled. In the trash the clock holds.
	newSLA("B3", `record.title.startsWith("WF9 B") && record.priority >= 3`, 480, "")
	newSLA("B1", `record.title.startsWith("WF9 B") && record.priority <= 2`, 60, "")
	now = time.Now().UTC()
	b := create("probe.probe", `{"title": "WF9 B `+suffix+`", "priority": 3}`)
	created, _ := time.Parse(time.RFC3339Nano, b["created_at"].(string))
	if cs := clocks(b["id"].(string)); len(cs) != 1 || cs["B3"].State != sla.StateRunning {
		t.Fatalf("priority 3: %+v", cs)
	}
	now = created.Add(90 * time.Minute)
	if r = call(t, api, http.MethodPatch, base+"/"+b["id"].(string), `{"priority": 1}`, map[string]string{"If-Match": `"1"`}); r.code != 200 {
		t.Fatalf("raise: %d %s", r.code, r.raw)
	}
	cs := clocks(b["id"].(string))
	if cs["B3"].State != sla.StateCancelled || !near(cs["B3"].Elapsed, 90) {
		t.Fatalf("priority 3 after raising: %+v", cs["B3"])
	}
	if c = cs["B1"]; c.State != sla.StateRunning || !c.Breached || !near(c.Elapsed, 90) || !c.StartedAt.Equal(created.Truncate(time.Microsecond)) ||
		c.BreachedAt.Sub(created.Add(time.Hour)).Abs() > time.Millisecond {
		t.Fatalf("priority 1 after raising: %+v (created %v)", c, created)
	}
	now = created.Add(100 * time.Minute)
	if r = call(t, api, http.MethodDelete, base+"/"+b["id"].(string), "", nil); r.code != 204 {
		t.Fatalf("trash: %d %s", r.code, r.raw)
	}
	now = created.Add(130 * time.Minute)
	if r = call(t, api, http.MethodPost, base+"/"+b["id"].(string)+":restore", "", nil); r.code != 204 && r.code != 200 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	now = created.Add(140 * time.Minute)
	if c = clocks(b["id"].(string))["B1"]; c.State != sla.StateRunning || !near(c.Elapsed, 110) {
		t.Fatalf("after the trash: %+v", c)
	}

	// C: 120 minutes in a calendar Monday to Friday 8–16 Berlin. Created on Friday 6 November
	// 2026 at 15:00, due on Monday at 9:00; on Saturday at noon there are still 60 minutes left.
	cal := create("core.calendar", `{"name": "WF9 Büro `+suffix+`"}`)["id"].(string)
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri"} {
		create("core.calendar_hours", `{"calendar": "`+cal+`", "weekday": "`+day+`", "starts": "08:00", "ends": "16:00"}`)
	}
	newSLA("C", `record.title.startsWith("WF9 C")`, 120, `, "calendar": "`+cal+`"`)
	berlin, _ := time.LoadLocation("Europe/Berlin")
	now = time.Date(2026, 11, 6, 15, 0, 0, 0, berlin)
	cc := create("probe.probe", `{"title": "WF9 C `+suffix+`"}`)["id"].(string)
	if c = clocks(cc)["C"]; c.Calendar == nil || *c.Calendar != cal || !c.DueAt.Equal(time.Date(2026, 11, 9, 9, 0, 0, 0, berlin)) {
		t.Fatalf("with a calendar: %+v", c)
	}
	now = time.Date(2026, 11, 7, 12, 0, 0, 0, berlin)
	if c = clocks(cc)["C"]; c.Remaining != 60 {
		t.Fatalf("on Saturday: %+v", c)
	}

	// The purge takes the clocks with it.
	call(t, api, http.MethodDelete, base+"/"+a, "", nil)
	call(t, api, http.MethodPost, base+"/"+a+":purge", "", nil)
	var left int
	if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM clock WHERE record_id = $1", a).Scan(&left); err != nil || left != 0 {
		t.Fatalf("clocks after the purge: %d %v", left, err)
	}
}

// The Prüfweg of WF10: escalation levels are rules with the trigger clock. At 80 % of the
// deadline level 1 raises the priority, at 100 % level 2 notifies the management group; a level
// at 150 % keeps a job waiting after the breach. Each level fires once, the history shows it.
// Wrong levels are refused.
func TestEscalationOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	now := time.Now().UTC().Truncate(time.Second)
	api.sla.Now = func() time.Time { return now }
	const base = "/api/v1/records/probe.probe"
	levels := []string{"wf10_s1", "wf10_s2", "wf10_s3", "wf10_bad"}
	var made []string
	cleanup := func() {
		for _, name := range levels {
			call(t, api, http.MethodDelete, "/api/v1/rules/"+name, "", nil)
		}
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}
	cleanup()
	defer cleanup()
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
	gid := create("core.group", `{"name": "WF10 Leitung `+suffix+`"}`)
	slaID := create("core.sla", `{"name": "WF10 Lösung `+suffix+`", "target_type": "probe.probe", "target_minutes": 60,
		"applies": "record.title.startsWith(\"WF10 `+suffix+`\")", "stop": "record.state == \"resolved\""}`)
	other := create("core.sla", `{"name": "WF10 Personen `+suffix+`", "target_type": "core.person", "target_minutes": 60, "stop": "false"}`)
	put := func(name, body string) resp {
		t.Helper()
		return call(t, api, http.MethodPut, "/api/v1/rules/"+name, body, nil)
	}
	level := func(percent int, sla, mode, action string) string {
		return fmt.Sprintf(`{"type_name": "probe.probe", "trigger": "clock", "mode": %q, "sla": %q, "percent": %d, "actions": [%s]}`,
			mode, sla, percent, action)
	}

	// Wrong levels: no share, a share above 1000, before the write, an SLA of another type, an
	// SLA on a rule that is no level.
	for _, body := range []string{
		level(0, slaID, "after", `{"set": {"priority": 1}}`),
		level(1001, slaID, "after", `{"set": {"priority": 1}}`),
		level(80, slaID, "before", `{"set": {"priority": 1}}`),
		level(80, other, "after", `{"set": {"priority": 1}}`),
		`{"type_name": "probe.probe", "trigger": "create", "mode": "after", "sla": "` + slaID + `", "actions": [{"set": {"priority": 1}}]}`,
	} {
		if r := put("wf10_bad", body); r.code != 422 {
			t.Fatalf("wrong level %s: %d %s", body, r.code, r.raw)
		}
	}
	if r := put("wf10_s1", level(80, slaID, "after", `{"set": {"priority": 1}}`)); r.code != 201 || r.body["percent"] != float64(80) || r.body["sla"] != slaID {
		t.Fatalf("level 1: %d %s", r.code, r.raw)
	}
	if r := put("wf10_s2", level(100, slaID, "after", `{"notify": {"to": ["core.group:`+gid+`"], "message": "Frist verletzt"}}`)); r.code != 201 {
		t.Fatalf("level 2: %d %s", r.code, r.raw)
	}
	if r := put("wf10_s3", level(150, slaID, "after", `{"notify": {"to": ["core.group:`+gid+`"], "message": "Frist weit überschritten"}}`)); r.code != 201 {
		t.Fatalf("level 3: %d %s", r.code, r.raw)
	}

	t0 := now
	id := create("probe.probe", `{"title": "WF10 `+suffix+` Drucker", "priority": 3}`)
	type clockRow struct {
		id     uuid.UUID
		gen    int64
		levels []string
	}
	clock := func() clockRow {
		t.Helper()
		var c clockRow
		if err := api.pool.QueryRow(ctx, "SELECT id, generation, levels FROM clock WHERE record_id = $1", id).Scan(&c.id, &c.gen, &c.levels); err != nil {
			t.Fatal(err)
		}
		return c
	}
	// The job of a clock waits for its next threshold, not for the deadline.
	nextJob := func(c clockRow) time.Time {
		t.Helper()
		var at time.Time
		if err := api.pool.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE queue = $1 AND args->>'clock' = $2
			AND (args->>'generation')::bigint = $3`, slaTestQueue, c.id.String(), c.gen).Scan(&at); err != nil {
			t.Fatalf("job of generation %d: %v", c.gen, err)
		}
		return at
	}
	c := clock()
	if at := nextJob(c); !at.Equal(t0.Add(48 * time.Minute)) {
		t.Fatalf("first job at %v, want 80 %% = T0+48", at)
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !ok() {
			if err := api.ruleEvents.Dispatch(ctx); err != nil {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// T0+49: level 1 fires once and raises the priority; the next job waits for the deadline.
	now = t0.Add(49 * time.Minute)
	if breached, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen}); err != nil || breached {
		t.Fatalf("job at 80 %%: %v %v", breached, err)
	}
	if c = clock(); len(c.levels) != 1 || c.levels[0] != "wf10_s1" || !nextJob(c).Equal(t0.Add(time.Hour)) {
		t.Fatalf("after level 1: %+v", c)
	}
	waitFor("priority 1 from level 1", func() bool {
		return call(t, api, http.MethodGet, base+"/"+id, "", nil).body["priority"] == float64(1)
	})

	// T0+61: past the deadline; level 2 fires and notifies the management; the job of level 3
	// waits for 150 % = T0+90.
	now = t0.Add(61 * time.Minute)
	if breached, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen}); err != nil || !breached {
		t.Fatalf("job at 100 %%: %v %v", breached, err)
	}
	if c = clock(); len(c.levels) != 2 || c.levels[1] != "wf10_s2" || !nextJob(c).Equal(t0.Add(90*time.Minute)) {
		t.Fatalf("after level 2: %+v", c)
	}
	waitFor("the notification of level 2", func() bool {
		return strings.Contains(call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil).raw, "Frist verletzt")
	})
	views := call(t, api, http.MethodGet, base+"/"+id+"/clocks", "", nil)
	if !strings.Contains(views.raw, `"levels":["wf10_s1","wf10_s2"]`) || !strings.Contains(views.raw, `"breached":true`) {
		t.Fatalf("clocks: %s", views.raw)
	}
	// The history shows each level once, with its rule and share, and the breach.
	h := call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil)
	var hist []map[string]any
	_ = json.Unmarshal([]byte(h.raw), &hist)
	count := map[string]int{}
	for _, e := range hist {
		if a, _ := e["action"].(string); a == "escalation" || a == "breach" {
			raw, _ := json.Marshal(e)
			for _, name := range []string{"wf10_s1", "wf10_s2", "wf10_s3"} {
				if strings.Contains(string(raw), `"`+name+`"`) {
					count[name]++
				}
			}
			count[a]++
		}
	}
	if count["escalation"] != 2 || count["breach"] != 1 || count["wf10_s1"] != 1 || count["wf10_s2"] != 1 || count["wf10_s3"] != 0 {
		t.Fatalf("history: %v %s", count, h.raw)
	}
	// An old job again does nothing.
	if _, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen - 1}); err != nil || len(clock().levels) != 2 {
		t.Fatalf("old job: %v %+v", err, clock())
	}
}

// The Prüfweg of WF11: the reason to wait "Rückmeldung Nutzer" with two reminders and a
// solution after 7 days, as an SLA that counts from the moment the record waits for it, with
// three escalation levels. The reminders go to the requester after 2 and 5 days; after 7 days
// the record is resolved with a text. A wait that ends and starts again counts anew.
func TestWaitReasonOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	now := time.Now().UTC().Truncate(time.Second)
	api.sla.Now = func() time.Time { return now }
	const base = "/api/v1/records/probe.probe"
	names := []string{"wf11_r1", "wf11_r2", "wf11_end", "wf11_bad"}
	var made []string
	cleanup := func() {
		for _, name := range names {
			call(t, api, http.MethodDelete, "/api/v1/rules/"+name, "", nil)
		}
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}
	cleanup()
	defer cleanup()
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
	transition := func(id, body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, base+"/"+id, "", nil).body["version"]
		if r := call(t, api, http.MethodPost, base+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
	}
	reason := create("core.wait_reason", `{"name": "WF11 Rückmeldung Nutzer `+suffix+`"}`)
	person := create("core.person", `{"last_name": "WF11 Anforderer `+suffix+`"}`)
	slaID := create("core.sla", `{"name": "WF11 Warten `+suffix+`", "target_type": "probe.probe", "target_minutes": 10080,
		"start_from": "applies", "applies": "record.state == \"waiting\" && record.wait_reason == \"`+reason+`\"", "stop": "false"}`)
	put := func(name, body string) resp {
		t.Helper()
		return call(t, api, http.MethodPut, "/api/v1/rules/"+name, body, nil)
	}
	level := func(point, action string) string {
		return `{"type_name": "probe.probe", "trigger": "clock", "mode": "after", "sla": "` + slaID + `", ` + point + `, "actions": [` + action + `]}`
	}
	remind := func(n int) string {
		return fmt.Sprintf(`{"notify": {"to": ["actors.requester"], "message": "Bitte melden Sie sich (%d)"}}`, n)
	}

	// Wrong levels: percent and minutes together, neither, a transition the type does not have.
	for _, body := range []string{
		level(`"percent": 50, "minutes": 10`, remind(1)),
		`{"type_name": "probe.probe", "trigger": "clock", "mode": "after", "sla": "` + slaID + `", "actions": [` + remind(1) + `]}`,
		level(`"minutes": 10`, `{"transition": {"name": "vanish"}}`),
	} {
		if r := put("wf11_bad", body); r.code != 422 {
			t.Fatalf("wrong level %s: %d %s", body, r.code, r.raw)
		}
	}
	if r := put("wf11_r1", level(`"minutes": 2880`, remind(1))); r.code != 201 || r.body["minutes"] != float64(2880) {
		t.Fatalf("reminder 1: %d %s", r.code, r.raw)
	}
	if r := put("wf11_r2", level(`"minutes": 7200`, remind(2))); r.code != 201 {
		t.Fatalf("reminder 2: %d %s", r.code, r.raw)
	}
	if r := put("wf11_end", level(`"percent": 100`, `{"transition": {"name": "resolve", "fields": {"solution": "Ohne Rückmeldung nach 7 Tagen gelöst"}}}`)); r.code != 201 {
		t.Fatalf("solution: %d %s", r.code, r.raw)
	}

	type clockRow struct {
		id      uuid.UUID
		state   string
		gen     int64
		levels  []string
		started time.Time
	}
	clockOf := func(id string) clockRow {
		t.Helper()
		var c clockRow
		if err := api.pool.QueryRow(ctx, "SELECT id, state, generation, levels, started_at FROM clock WHERE record_id = $1", id).
			Scan(&c.id, &c.state, &c.gen, &c.levels, &c.started); err != nil {
			t.Fatal(err)
		}
		return c
	}
	nextJob := func(c clockRow) time.Time {
		t.Helper()
		var at time.Time
		if err := api.pool.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE queue = $1 AND args->>'clock' = $2
			AND (args->>'generation')::bigint = $3`, slaTestQueue, c.id.String(), c.gen).Scan(&at); err != nil {
			t.Fatalf("job of generation %d: %v", c.gen, err)
		}
		return at
	}
	due := func(c clockRow) {
		t.Helper()
		if _, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen}); err != nil {
			t.Fatal(err)
		}
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !ok() {
			if err := api.ruleEvents.Dispatch(ctx); err != nil {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	history := func(id string) string { return call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil).raw }
	day := 24 * time.Hour

	// Created at T0, waiting for the requester from T0+1h: the clock counts from then, the
	// first reminder is due two days later.
	t0 := now
	id := create("probe.probe", `{"title": "WF11 `+suffix+` VPN", "actors": {"requester": [{"type": "core.person", "id": "`+person+`"}]}}`)
	transition(id, `{"transition": "start"}`)
	now = t0.Add(time.Hour)
	transition(id, `{"transition": "wait", "fields": {"wait_reason": "`+reason+`"}}`)
	c := clockOf(id)
	if c.state != sla.StateRunning || !c.started.Equal(now) || !nextJob(c).Equal(now.Add(2*day)) {
		t.Fatalf("waiting: %+v", c)
	}
	w0 := now

	// Two days later the first reminder, five days later the second.
	now = w0.Add(2*day + time.Minute)
	due(c)
	waitFor("reminder 1", func() bool { return strings.Contains(history(id), "Bitte melden Sie sich (1)") })
	if c = clockOf(id); len(c.levels) != 1 || !nextJob(c).Equal(w0.Add(5*day)) {
		t.Fatalf("after reminder 1: %+v", c)
	}
	now = w0.Add(5*day + time.Minute)
	due(c)
	waitFor("reminder 2", func() bool { return strings.Contains(history(id), "Bitte melden Sie sich (2)") })
	if c = clockOf(id); len(c.levels) != 2 || !nextJob(c).Equal(w0.Add(7*day)) {
		t.Fatalf("after reminder 2: %+v", c)
	}
	if h := history(id); !strings.Contains(h, person) {
		t.Fatalf("the reminder does not go to the requester: %s", h)
	}
	// Seven days later the record is resolved with the text; the wait is over, its clock ends.
	now = w0.Add(7*day + time.Minute)
	due(c)
	waitFor("the solution after 7 days", func() bool {
		return call(t, api, http.MethodGet, base+"/"+id, "", nil).body["state"] == "resolved"
	})
	if rec := call(t, api, http.MethodGet, base+"/"+id, "", nil); rec.body["solution"] != "Ohne Rückmeldung nach 7 Tagen gelöst" {
		t.Fatalf("solution: %s", rec.raw)
	}
	if c = clockOf(id); c.state != sla.StateCancelled {
		t.Fatalf("clock after the solution: %+v", c)
	}

	// A wait that ends and starts again counts anew: three days, back to work, a day later
	// waiting again; the reminders start over.
	now = t0
	id2 := create("probe.probe", `{"title": "WF11 `+suffix+` Drucker", "actors": {"requester": [{"type": "core.person", "id": "`+person+`"}]}}`)
	transition(id2, `{"transition": "start"}`)
	transition(id2, `{"transition": "wait", "fields": {"wait_reason": "`+reason+`"}}`)
	now = t0.Add(3 * day)
	c = clockOf(id2)
	due(c)
	if c = clockOf(id2); len(c.levels) != 1 {
		t.Fatalf("first wait: %+v", c)
	}
	transition(id2, `{"transition": "resume"}`)
	if c = clockOf(id2); c.state != sla.StateCancelled {
		t.Fatalf("back to work: %+v", c)
	}
	now = t0.Add(4 * day)
	transition(id2, `{"transition": "wait"}`)
	if c = clockOf(id2); c.state != sla.StateRunning || len(c.levels) != 0 || !c.started.Equal(now) || !nextJob(c).Equal(now.Add(2*day)) {
		t.Fatalf("second wait: %+v", c)
	}
}

// The Prüfweg of WF12: time rules of the state model are SLAs with levels. A record open for
// more than 3 days warns its assigned group; a resolved record is closed 7 days after the
// solution. Both count round the clock, although the unit has a calendar of working hours.
func TestTimeRulesOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	now := time.Now().UTC().Truncate(time.Second)
	api.sla.Now = func() time.Time { return now }
	const base = "/api/v1/records/probe.probe"
	names := []string{"wf12_warn", "wf12_close"}
	var made []string
	cleanup := func() {
		for _, name := range names {
			call(t, api, http.MethodDelete, "/api/v1/rules/"+name, "", nil)
		}
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
		call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.wf12", "", nil)
	}
	cleanup()
	defer cleanup()
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
	transition := func(id, body string) {
		t.Helper()
		v := call(t, api, http.MethodGet, base+"/"+id, "", nil).body["version"]
		if r := call(t, api, http.MethodPost, base+"/"+id+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)}); r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
	}
	// The unit works Monday to Friday 8–16; the time rules ignore that.
	cal := create("core.calendar", `{"name": "WF12 Büro `+suffix+`"}`)
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri"} {
		create("core.calendar_hours", `{"calendar": "`+cal+`", "weekday": "`+day+`", "starts": "08:00", "ends": "16:00"}`)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "wf12"}`, nil); r.code != 201 {
		t.Fatalf("node: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodPut, "/api/v1/org/nodes/root.wf12/settings/calendar.default", `"`+cal+`"`, nil); r.code != 200 && r.code != 204 {
		t.Fatalf("setting: %d %s", r.code, r.raw)
	}
	group := create("core.group", `{"name": "WF12 Netz `+suffix+`"}`)
	title := "WF12 " + suffix
	warnSLA := create("core.sla", `{"name": "WF12 Laufzeit `+suffix+`", "target_type": "probe.probe", "target_minutes": 4320,
		"counts": "round_the_clock", "applies": "record.title.startsWith(\"`+title+`\")", "stop": "record.state in [\"resolved\", \"closed\"]"}`)
	closeSLA := create("core.sla", `{"name": "WF12 Schließen `+suffix+`", "target_type": "probe.probe", "target_minutes": 10080,
		"counts": "round_the_clock", "start_from": "applies",
		"applies": "record.state == \"resolved\" && record.title.startsWith(\"`+title+`\")", "stop": "false"}`)
	put := func(name, body string) {
		t.Helper()
		if r := call(t, api, http.MethodPut, "/api/v1/rules/"+name, body, nil); r.code != 201 {
			t.Fatalf("rule %s: %d %s", name, r.code, r.raw)
		}
	}
	put("wf12_warn", `{"type_name": "probe.probe", "trigger": "clock", "mode": "after", "sla": "`+warnSLA+`", "percent": 100,
		"actions": [{"notify": {"to": ["actors.assigned"], "message": "Vorgang läuft zu lange"}}]}`)
	put("wf12_close", `{"type_name": "probe.probe", "trigger": "clock", "mode": "after", "sla": "`+closeSLA+`", "percent": 100,
		"actions": [{"transition": {"name": "close"}}]}`)

	type clockRow struct {
		id      uuid.UUID
		state   string
		gen     int64
		cal     *uuid.UUID
		started time.Time
	}
	clockOf := func(id, sla string) clockRow {
		t.Helper()
		var c clockRow
		if err := api.pool.QueryRow(ctx, "SELECT id, state, generation, calendar, started_at FROM clock WHERE record_id = $1 AND sla = $2", id, sla).
			Scan(&c.id, &c.state, &c.gen, &c.cal, &c.started); err != nil {
			t.Fatal(err)
		}
		return c
	}
	nextJob := func(c clockRow) time.Time {
		t.Helper()
		var at time.Time
		if err := api.pool.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE queue = $1 AND args->>'clock' = $2
			AND (args->>'generation')::bigint = $3`, slaTestQueue, c.id.String(), c.gen).Scan(&at); err != nil {
			t.Fatalf("job of generation %d: %v", c.gen, err)
		}
		return at
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !ok() {
			if err := api.ruleEvents.Dispatch(ctx); err != nil {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	get := func(id string) resp { return call(t, api, http.MethodGet, base+"/"+id, "", nil) }
	day := 24 * time.Hour

	// Created on Friday 6 November 2026 at 15:00 in the unit with office hours: the warning is
	// due three days later, on Monday at 15:00, not after three working days.
	berlin, _ := time.LoadLocation("Europe/Berlin")
	t0 := time.Date(2026, 11, 6, 15, 0, 0, 0, berlin)
	now = t0
	id := create("probe.probe", `{"title": "`+title+` Switch", "org_path": "root.wf12",
		"actors": {"assigned": [{"type": "core.group", "id": "`+group+`"}]}}`)
	transition(id, `{"transition": "start"}`)
	c := clockOf(id, warnSLA)
	if c.state != sla.StateRunning || c.cal != nil || !nextJob(c).Equal(t0.Add(3*day)) {
		t.Fatalf("warning clock: %+v", c)
	}
	now = t0.Add(3*day + time.Minute)
	if _, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen}); err != nil {
		t.Fatal(err)
	}
	waitFor("the warning", func() bool {
		h := call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil).raw
		return strings.Contains(h, "Vorgang läuft zu lange") && strings.Contains(h, group)
	})

	// Resolved on day 4: the warning clock stops, the closing clock counts from then and is
	// due seven days later; then the record is closed.
	now = t0.Add(4 * day)
	transition(id, `{"transition": "resolve", "fields": {"solution": "Port getauscht"}}`)
	if c = clockOf(id, warnSLA); c.state != sla.StateStopped {
		t.Fatalf("warning clock after the solution: %+v", c)
	}
	c = clockOf(id, closeSLA)
	if c.state != sla.StateRunning || !c.started.Equal(now) || !nextJob(c).Equal(now.Add(7*day)) {
		t.Fatalf("closing clock: %+v", c)
	}
	now = t0.Add(11*day + time.Minute)
	if _, err := api.sla.Due(ctx, sla.DueArgs{Clock: c.id, Generation: c.gen}); err != nil {
		t.Fatal(err)
	}
	waitFor("the closing", func() bool { return get(id).body["state"] == "closed" })
	if c = clockOf(id, closeSLA); c.state != sla.StateCancelled {
		t.Fatalf("closing clock after closing: %+v", c)
	}
}
