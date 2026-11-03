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
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/sla"
)

// The deadline at the record for its list (UI34, UI-Plan E37): the clocks leave the next
// deadline and the three-quarter point at the record without a new version; the list sorts by
// it with the records without one last, page by page; the query filters breached, close and
// due before a moment, also against "@now"; a pause leaves the record paused; the job of the
// deadline keeps it; the types say which ones have clocks.
func TestDeadlineOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	now := time.Now().UTC().Truncate(time.Second)
	api.sla.Now = func() time.Time { return now }
	const base = "/api/v1/records/probe.probe"
	title := "UI34 " + suffix
	var made []string
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
	moment := func(v any) time.Time {
		t.Helper()
		s, _ := v.(string)
		at, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatalf("not a moment: %v", v)
		}
		return at
	}
	for _, s := range []struct {
		name     string
		priority int
		minutes  int
	}{{"D", 3, 60}, {"E", 1, 20}} {
		applies, _ := json.Marshal(fmt.Sprintf("record.title == %q && record.priority == %d", title, s.priority))
		create("core.sla", fmt.Sprintf(`{"name": "UI34 %s %s", "target_type": "probe.probe", "target_minutes": %d,
			"applies": %s, "pause": "record.state == \"waiting\"",
			"stop": "record.state in [\"resolved\", \"closed\"]"}`, s.name, suffix, s.minutes, applies))
	}

	// D runs 60 minutes, E 20, F has no SLA. The answer of the write shows the deadline.
	t0 := now
	d := create("probe.probe", `{"title": "`+title+`", "priority": 3}`)
	if !moment(d["sla_due_at"]).Equal(t0.Add(time.Hour)) || !moment(d["sla_warn_at"]).Equal(t0.Add(45*time.Minute)) ||
		d["sla_paused"] != false || d["version"] != float64(1) {
		t.Fatalf("deadline of D: %v %v %v version %v", d["sla_due_at"], d["sla_warn_at"], d["sla_paused"], d["version"])
	}
	e := create("probe.probe", `{"title": "`+title+`", "priority": 1}`)
	f := create("probe.probe", `{"title": "`+title+`", "priority": 5}`)
	g := create("probe.probe", `{"title": "`+title+`", "priority": 4}`)
	if f["sla_due_at"] != nil || f["sla_warn_at"] != nil || f["sla_paused"] != false {
		t.Fatalf("F without an SLA: %v %v %v", f["sla_due_at"], f["sla_warn_at"], f["sla_paused"])
	}

	// The list by deadline, one per page: E, D, then F and G without one, by id; backwards the
	// ones without stay last.
	order := func(sort string) []string {
		t.Helper()
		var ids []string
		next := ""
		for range 6 {
			q := base + "?limit=1&sort=" + sort + "&filter[title]=" + url.QueryEscape(title)
			if next != "" {
				q += "&after=" + next
			}
			r := call(t, api, http.MethodGet, q, "", nil)
			if r.code != 200 {
				t.Fatalf("list by %s: %d %s", sort, r.code, r.raw)
			}
			for _, it := range r.body["items"].([]any) {
				ids = append(ids, it.(map[string]any)["id"].(string))
			}
			next, _ = r.body["next"].(string)
			if next == "" {
				break
			}
		}
		return ids
	}
	id := func(rec map[string]any) string { return rec["id"].(string) }
	low, high := id(f), id(g)
	if high < low {
		low, high = high, low
	}
	if got, want := order("sla_due_at"), []string{id(e), id(d), low, high}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("by deadline: %v, want %v", got, want)
	}
	if got, want := order("-sla_due_at"), []string{id(d), id(e), high, low}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("by deadline, latest first: %v, want %v", got, want)
	}

	// The filters of the list go through the query: breached, close, due before a moment.
	found := func(where string) []string {
		t.Helper()
		body := `{"where": {"and": [{"field": "title", "eq": "` + title + `"}, ` + where + `]}, "sort": ["sla_due_at"], "fields": ["title"]}`
		r := call(t, api, http.MethodPost, base+":query", body, nil)
		if r.code != 200 {
			t.Fatalf("query %s: %d %s", where, r.code, r.raw)
		}
		var ids []string
		for _, it := range r.body["items"].([]any) {
			ids = append(ids, it.(map[string]any)["id"].(string))
		}
		return ids
	}
	at := func(m time.Time) string { return m.Format(time.RFC3339) }
	// At T0+50: E is past its 20 minutes, D is close (past 45 of 60).
	t50 := t0.Add(50 * time.Minute)
	if got := found(`{"field": "sla_due_at", "lte": "` + at(t50) + `"}`); fmt.Sprint(got) != fmt.Sprint([]string{id(e)}) {
		t.Fatalf("breached at T0+50: %v", got)
	}
	closeAt := `{"field": "sla_warn_at", "lte": "` + at(t50) + `"}, {"field": "sla_due_at", "gt": "` + at(t50) + `"}`
	if got := found(closeAt); fmt.Sprint(got) != fmt.Sprint([]string{id(d)}) {
		t.Fatalf("close at T0+50: %v", got)
	}
	if got := found(`{"field": "sla_due_at", "lte": "` + at(t0.Add(2*time.Hour)) + `"}`); fmt.Sprint(got) != fmt.Sprint([]string{id(e), id(d)}) {
		t.Fatalf("due within two hours: %v", got)
	}

	// "@now" is the moment of the query, for saved searches: nothing is overdue yet in real time.
	if got := found(`{"field": "sla_due_at", "lte": "@now"}`); len(got) != 0 {
		t.Fatalf("overdue now: %v", got)
	}
	if got := found(`{"field": "sla_due_at", "gt": "@now"}`); fmt.Sprint(got) != fmt.Sprint([]string{id(e), id(d)}) {
		t.Fatalf("due after now: %v", got)
	}

	// The job of E's deadline marks the breach; the deadline stays.
	var clock uuid.UUID
	var gen int64
	if err := api.pool.QueryRow(ctx, "SELECT id, generation FROM clock WHERE record_id = $1", id(e)).Scan(&clock, &gen); err != nil {
		t.Fatal(err)
	}
	now = t0.Add(21 * time.Minute)
	if breached, err := api.sla.Due(ctx, sla.DueArgs{Clock: clock, Generation: gen}); err != nil || !breached {
		t.Fatalf("job at E's deadline: %v %v", breached, err)
	}
	re := call(t, api, http.MethodGet, base+"/"+id(e), "", nil).body
	if !moment(re["sla_due_at"]).Equal(t0.Add(20*time.Minute)) || re["version"] != float64(1) {
		t.Fatalf("E after the breach: %v version %v", re["sla_due_at"], re["version"])
	}

	// D waits: no deadline, paused; going on gives the deadline back, moved by the pause.
	transition := func(rec map[string]any, body string) map[string]any {
		t.Helper()
		v := call(t, api, http.MethodGet, base+"/"+id(rec), "", nil).body["version"]
		r := call(t, api, http.MethodPost, base+"/"+id(rec)+":transition", body, map[string]string{"If-Match": fmt.Sprintf(`"%v"`, v)})
		if r.code != 200 {
			t.Fatalf("transition %s: %d %s", body, r.code, r.raw)
		}
		return r.body
	}
	now = t0.Add(10 * time.Minute)
	transition(d, `{"transition": "start"}`)
	w := transition(d, `{"transition": "wait"}`)
	if w["sla_due_at"] != nil || w["sla_warn_at"] != nil || w["sla_paused"] != true {
		t.Fatalf("D waiting: %v %v %v", w["sla_due_at"], w["sla_warn_at"], w["sla_paused"])
	}
	now = t0.Add(40 * time.Minute)
	on := transition(d, `{"transition": "resume"}`)
	if !moment(on["sla_due_at"]).Equal(t0.Add(90*time.Minute)) || !moment(on["sla_warn_at"]).Equal(t0.Add(75*time.Minute)) || on["sla_paused"] != false {
		t.Fatalf("D going on: %v %v %v", on["sla_due_at"], on["sla_warn_at"], on["sla_paused"])
	}
	// The types say which ones have clocks, for the column of the list.
	var types []map[string]any
	if r := call(t, api, http.MethodGet, "/api/v1/types", "", nil); r.code != 200 || json.Unmarshal([]byte(r.raw), &types) != nil {
		t.Fatalf("types: %d %s", r.code, r.raw)
	}
	timed := map[string]bool{}
	for _, ty := range types {
		timed[fmt.Sprintf("%v.%v", ty["package"], ty["name"])] = ty["timed"] == true
	}
	if !timed["probe.probe"] || timed["core.person"] {
		t.Fatalf("timed types: %v", timed)
	}

	// The deadline is no change of the record: the history holds only the transitions.
	h := call(t, api, http.MethodGet, base+"/"+id(d)+"/history", "", nil)
	if h.code != 200 || strings.Contains(h.raw, "sla_due_at") {
		t.Fatalf("the history shows the deadline: %s", h.raw)
	}
}
