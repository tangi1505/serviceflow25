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

	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// The Prüfweg of WF8: a calendar Monday to Friday 8–16 in Berlin; 8 working hours from Friday
// 15:00 end on Monday 15:00, a holiday on that Monday moves the end to Tuesday 15:00. The
// holidays of a state come in one call, twice gives nothing new; the setting calendar.default
// gives a unit its calendar; hours that are no times are refused when counting.
func TestCalendarsOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	var made []string // type/id to remove at the end
	create := func(typeName, body string) map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, "/api/v1/records/"+typeName, body, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", typeName, r.code, r.raw)
		}
		made = append(made, typeName+"/"+r.body["id"].(string))
		return r.body
	}
	cal := create("core.calendar", `{"name": "WF8 Büro `+suffix+`"}`)
	if cal["timezone"] != "Europe/Berlin" {
		t.Fatalf("time zone by default: %v", cal)
	}
	id := cal["id"].(string)
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri"} {
		create("core.calendar_hours", `{"calendar": "`+id+`", "weekday": "`+day+`", "starts": "08:00", "ends": "16:00"}`)
	}
	add := func(start string, minutes int) resp {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"start": start, "minutes": minutes})
		return call(t, api, http.MethodPost, "/api/v1/calendars/"+id+":add", string(body), nil)
	}

	if r := add("2026-11-06T15:00:00+01:00", 480); r.code != 200 || r.body["end"] != "2026-11-09T15:00:00+01:00" {
		t.Fatalf("Friday 15:00 + 8 h: %d %s", r.code, r.raw)
	}
	create("core.calendar_holiday", `{"calendar": "`+id+`", "date": "2026-11-09", "name": "Betriebsausflug"}`)
	if r := add("2026-11-06T15:00:00+01:00", 480); r.code != 200 || r.body["end"] != "2026-11-10T15:00:00+01:00" {
		t.Fatalf("with a holiday on Monday: %d %s", r.code, r.raw)
	}
	r := call(t, api, http.MethodPost, "/api/v1/calendars/"+id+":between", `{"from": "2026-11-06T15:00:00+01:00", "to": "2026-11-10T09:30:00+01:00"}`, nil)
	if r.code != 200 || r.body["minutes"] != float64(150) {
		t.Fatalf("between: %d %s", r.code, r.raw)
	}

	// The public holidays of NRW in 2026: eleven days, the second call adds none.
	if r = call(t, api, http.MethodPost, "/api/v1/calendars/"+id+":holidays", `{"year": 2026, "state": "NW"}`, nil); r.code != 200 || r.body["added"] != float64(11) {
		t.Fatalf("holidays: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/calendars/"+id+":holidays", `{"year": 2026, "state": "NW"}`, nil); r.code != 200 || r.body["added"] != float64(0) {
		t.Fatalf("holidays again: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/calendars/"+id+":holidays", `{"year": 2026, "state": "XX"}`, nil); r.code != 400 && r.code != 422 {
		t.Fatalf("unknown state: %d %s", r.code, r.raw)
	}
	// Thursday 24 December 2026 15:00 + 2 h: the 25th (a Friday) is a holiday of NRW now.
	if r = add("2026-12-24T15:00:00+01:00", 120); r.code != 200 || r.body["end"] != "2026-12-28T09:00:00+01:00" {
		t.Fatalf("over Christmas: %d %s", r.code, r.raw)
	}

	// The setting calendar.default gives a unit its calendar.
	call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.wf8", "", nil)
	if r = call(t, api, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "wf8"}`, nil); r.code != 201 {
		t.Fatalf("node: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPut, "/api/v1/org/nodes/root.wf8/settings/calendar.default", `"`+id+`"`, nil); r.code != 200 && r.code != 204 {
		t.Fatalf("setting: %d %s", r.code, r.raw)
	}
	svc := calendar.New(records.New(api.pool), org.New(api.pool).Setting)
	c, got, err := svc.ForNode(ctx, "root.wf8")
	if err != nil || c == nil || got.String() != id {
		t.Fatalf("calendar of the unit: %v %v", got, err)
	}
	if c, _, err := svc.ForNode(ctx, "root"); err != nil || c != nil {
		t.Fatalf("no setting, no calendar: %v %v", c, err)
	}

	// Hours that are no times: counting is refused with 422; an unknown calendar is 404.
	create("core.calendar_hours", `{"calendar": "`+id+`", "weekday": "sat", "starts": "8", "ends": "12:00"}`)
	if r = add("2026-11-06T15:00:00+01:00", 60); r.code != 422 || !strings.Contains(r.raw, "working hours") {
		t.Fatalf("bad hours: %d %s", r.code, r.raw)
	}
	if r = call(t, api, http.MethodPost, "/api/v1/calendars/"+uuid.NewString()+":add", `{"start": "2026-11-06T15:00:00Z", "minutes": 1}`, nil); r.code != 404 {
		t.Fatalf("unknown calendar: %d %s", r.code, r.raw)
	}

	// Clean up: the generated holidays, then what the test made, the calendar last.
	page := call(t, api, http.MethodGet, "/api/v1/records/core.calendar_holiday?limit=100&filter[calendar]="+id, "", nil)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal([]byte(page.raw), &list)
	for _, h := range list.Items {
		made = append(made, "core.calendar_holiday/"+h["id"].(string))
	}
	seen := map[string]bool{}
	for i := len(made) - 1; i >= 0; i-- {
		if seen[made[i]] {
			continue
		}
		seen[made[i]] = true
		typ, rid, _ := strings.Cut(made[i], "/")
		call(t, api, http.MethodDelete, "/api/v1/records/"+typ+"/"+rid, "", nil)
		call(t, api, http.MethodPost, "/api/v1/records/"+typ+"/"+rid+":purge", "", nil)
	}
	call(t, api, http.MethodDelete, "/api/v1/org/nodes/root.wf8", "", nil)
}
