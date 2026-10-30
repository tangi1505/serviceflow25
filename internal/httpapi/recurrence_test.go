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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The Prüfweg of WF14: a recurrence "every Monday at 7" creates exactly one record per Monday
// in the test's time: the first run only finds the next date, before 7 nothing happens, at 7
// one record with the week in its title and a due date three days later; two instances at the
// same minute still make one; Mondays missed while nothing ran are made up once. Wrong
// recurrences are refused.
func TestRecurrenceOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	title := "WF14 " + suffix
	var made []string
	defer func() {
		for _, item := range created(t, api, title) {
			made = append(made, "probe.probe/"+item["id"].(string))
		}
		for i := len(made) - 1; i >= 0; i-- {
			typ, id, _ := strings.Cut(made[i], "/")
			call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+id, "", nil)
			call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+id+":purge", "", nil)
		}
	}()
	recurrence := func(name, target, schedule, tz, template string) resp {
		body, _ := json.Marshal(map[string]any{"name": name, "target_type": target, "schedule": schedule, "timezone": tz, "template": template})
		return call(t, api, http.MethodPost, "/api/v1/records/core.recurrence", string(body), nil)
	}
	good := `{"title": "` + title + ` KW {{week}}", "due_on": "+3d"}`
	for _, c := range []struct{ target, schedule, tz, template, field string }{
		{"probe.probe", "jeden Montag", "Europe/Berlin", good, "schedule"},
		{"probe.probe", "0 7 * * 1", "Mars/Base", good, "timezone"},
		{"probe.nothing", "0 7 * * 1", "Europe/Berlin", good, "target_type"},
		{"probe.probe", "0 7 * * 1", "Europe/Berlin", "kein JSON", "template"},
		{"probe.probe", "0 7 * * 1", "Europe/Berlin", `{"priority": 3}`, "template"},
		{"probe.probe", "0 7 * * 1", "Europe/Berlin", `{"title": "x", "nofield": 1}`, "template"},
	} {
		if r := recurrence("WF14 falsch "+suffix, c.target, c.schedule, c.tz, c.template); r.code != 422 || !strings.Contains(r.raw, `"`+c.field+`"`) {
			t.Fatalf("wrong recurrence %+v: %d %s", c, r.code, r.raw)
		}
	}
	r := recurrence("WF14 Wochenprüfung "+suffix, "probe.probe", "0 7 * * 1", "Europe/Berlin", good)
	if r.code != 201 {
		t.Fatalf("recurrence: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)
	made = append(made, "core.recurrence/"+id)
	berlin, _ := time.LoadLocation("Europe/Berlin")
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, berlin) }
	run := func(now time.Time, want int) {
		t.Helper()
		if n, err := api.recur.Run(ctx, now); err != nil || n != want {
			t.Fatalf("run at %v: %d created, %v; want %d", now, n, err, want)
		}
	}
	nextRun := func() string {
		return fmt.Sprint(call(t, api, http.MethodGet, "/api/v1/records/core.recurrence/"+id, "", nil).body["next_run"])
	}
	week := func(day int) string { _, w := at(day, 7, 0).ISOWeek(); return fmt.Sprintf("%s KW %d", title, w) }

	// Sunday 8 November 2026: the first run finds the next date, Monday 9 at 7:00 Berlin.
	run(at(8, 12, 0), 0)
	if got := nextRun(); !strings.HasPrefix(got, "2026-11-09T06:00:00") {
		t.Fatalf("next date: %s", got)
	}
	run(at(9, 6, 59), 0)
	run(at(9, 7, 0).Add(30*time.Second), 1)
	items := created(t, api, title)
	if len(items) != 1 || items[0]["title"] != week(9) || items[0]["due_on"] != "2026-11-12" || items[0]["source"] != "recurrence:"+id {
		t.Fatalf("Monday 9: %v", items)
	}
	run(at(9, 7, 1), 0)
	if got := nextRun(); !strings.HasPrefix(got, "2026-11-16T06:00:00") {
		t.Fatalf("next date after the first: %s", got)
	}

	// Monday 16: two instances run at the same minute, one record.
	var wg sync.WaitGroup
	counts := make([]int, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { counts[i], errs[i] = api.recur.Run(ctx, at(16, 7, 0).Add(10*time.Second)) })
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || counts[0]+counts[1] != 1 {
		t.Fatalf("two instances: %v %v", counts, errs)
	}

	// Nothing ran on Monday 23; on Wednesday 25 that Monday is made up once, then Monday 30.
	run(at(25, 10, 0), 1)
	run(at(25, 10, 5), 0)
	if got := nextRun(); !strings.HasPrefix(got, "2026-11-30T06:00:00") {
		t.Fatalf("next date after making up: %s", got)
	}
	var titles []string
	for _, item := range created(t, api, title) {
		titles = append(titles, item["title"].(string))
	}
	if want := []string{week(9), week(16), week(23)}; strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("records: %v, want %v", titles, want)
	}
	if h := call(t, api, http.MethodGet, "/api/v1/records/core.recurrence/"+id+"/history", "", nil); !strings.Contains(h.raw, `"last_run"`) {
		t.Fatalf("history: %s", h.raw)
	}
}

// created returns the probe records whose title starts with prefix, oldest first.
func created(t *testing.T, api *testAPI, prefix string) []map[string]any {
	t.Helper()
	r := call(t, api, http.MethodPost, "/api/v1/records/probe.probe:query", `{"where": {"field": "title", "starts": "`+prefix+`"}, "sort": ["created_at"], "limit": 100}`, nil)
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if r.code != 200 || json.Unmarshal([]byte(r.raw), &page) != nil {
		t.Fatalf("query: %d %s", r.code, r.raw)
	}
	return page.Items
}
