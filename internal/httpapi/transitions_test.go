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
	"testing"
)

// The Prüfweg of WF1 over HTTP: a new probe starts in "new"; an allowed transition answers 200
// with the new state and writes an event into the stream; a transition from the wrong state
// or with a failed condition answers 409 with the reason; the state cannot be set directly.
func TestStateModelOverHTTP(t *testing.T) {
	api := apiServer(t)
	const base = "/api/v1/records/probe.probe"

	// Where the stream stands before this test (other tests and runs leave events behind).
	before := call(t, api, http.MethodGet, "/api/v1/events/position", "", nil)
	if before.code != 200 {
		t.Fatalf("position: %d %s", before.code, before.raw)
	}
	last := int64(before.body["position"].(float64))

	r := call(t, api, http.MethodPost, base, `{"title": "state"}`, nil)
	if r.code != 201 || r.body["state"] != "new" {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)

	// The state is a system field: PATCH refuses it.
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"state": "done"}`, map[string]string{"If-Match": `"1"`})
	if r.code != 422 {
		t.Fatalf("patch state: %d %s", r.code, r.raw)
	}

	// Transitions of a new record: start allowed, finish, wait, resume, reopen, resolve and close
	// not (state; resolve and close came with WF2, wait and resume with WF9).
	r = call(t, api, http.MethodGet, base+"/"+id+"/transitions", "", nil)
	if r.code != 200 {
		t.Fatalf("transitions: %d %s", r.code, r.raw)
	}
	var infos []map[string]any
	if err := json.Unmarshal([]byte(r.raw), &infos); err != nil || len(infos) != 7 {
		t.Fatalf("transitions: %v %s", err, r.raw)
	}
	if infos[0]["name"] != "start" || infos[0]["allowed"] != true || infos[1]["allowed"] != false || infos[1]["reason"] != "state" {
		t.Fatalf("transitions of a new record: %s", r.raw)
	}

	// Wrong state and unknown transition.
	r = call(t, api, http.MethodPost, base+"/"+id+":transition", `{"transition": "finish"}`, nil)
	if r.code != 409 || r.body["type"] != "/problems/conflict" || r.body["detail"] == nil {
		t.Fatalf("finish from new: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPost, base+"/"+id+":transition", `{"transition": "nope"}`, nil)
	if r.code != 422 {
		t.Fatalf("unknown transition: %d %s", r.code, r.raw)
	}

	// Allowed: start, finish, reopen (priority 3 <= 3), start, finish.
	for i, name := range []string{"start", "finish", "reopen", "start", "finish"} {
		r = call(t, api, http.MethodPost, base+"/"+id+":transition", `{"transition": "`+name+`"}`, nil)
		if r.code != 200 {
			t.Fatalf("%s: %d %s", name, r.code, r.raw)
		}
		want := map[string]string{"start": "in_progress", "finish": "done", "reopen": "new"}[name]
		if r.body["state"] != want || r.hdr.Get("ETag") == "" {
			t.Fatalf("%s: state %v, etag %q", name, r.body["state"], r.hdr.Get("ETag"))
		}
		if v := r.body["version"].(float64); int(v) != i+2 {
			t.Fatalf("%s: version %v, want %d", name, v, i+2)
		}
	}

	// The condition: with priority 5 reopen is refused, and the list says why.
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"priority": 5}`, map[string]string{"If-Match": `"6"`})
	if r.code != 200 {
		t.Fatalf("patch priority: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPost, base+"/"+id+":transition", `{"transition": "reopen"}`, nil)
	if r.code != 409 || r.body["detail"] == nil {
		t.Fatalf("reopen with priority 5: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodGet, base+"/"+id+"/transitions", "", nil)
	_ = json.Unmarshal([]byte(r.raw), &infos)
	// reopen comes fifth, after start, finish, and wait and resume of WF9.
	if infos[4]["name"] != "reopen" || infos[4]["allowed"] != false || infos[4]["reason"] != "condition" {
		t.Fatalf("reopen info: %s", r.raw)
	}

	// Five transition events in order for this record, with the actor of the session (the
	// create and update of the record are events of their own since PL24).
	r = call(t, api, http.MethodGet, "/api/v1/events?after="+jsonInt(last), "", nil)
	var all, events []map[string]any
	if err := json.Unmarshal([]byte(r.raw), &all); err != nil {
		t.Fatalf("events: %v %d %s", err, r.code, r.raw)
	}
	for _, e := range all {
		if e["kind"] == "transition" {
			events = append(events, e)
		}
	}
	if len(events) != 5 || len(all) != 7 {
		t.Fatalf("events: %d transitions of %d: %s", len(events), len(all), r.raw)
	}
	for i, e := range events {
		payload := e["payload"].(map[string]any)
		want := []string{"start", "finish", "reopen", "start", "finish"}[i]
		if e["kind"] != "transition" || e["record_id"] != id || payload["transition"] != want || e["actor"] == nil {
			t.Fatalf("event %d: %v", i, e)
		}
		if i > 0 && e["position"].(float64) <= events[i-1]["position"].(float64) {
			t.Fatalf("events out of order: %v", events)
		}
	}
	// And the history shows the transitions as audit rows.
	r = call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil)
	var hist []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &hist)
	transitions := 0
	for _, h := range hist {
		if h["action"] == "transition" && h["field"] == "state" {
			transitions++
		}
	}
	if transitions != 5 {
		t.Fatalf("history has %d transitions, want 5: %s", transitions, r.raw)
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
