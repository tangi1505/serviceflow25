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
)

// The Prüfweg of WF2 on probe.probe: resolving without a solution is 422 with the field name;
// the solution goes with the transition in one version; a closed record is read-only, and the
// form learns so from :field-states; a wish hides fields and is preset.
func TestFieldStatesOverHTTP(t *testing.T) {
	api := apiServer(t)
	const base = "/api/v1/records/probe.probe"
	transition := func(id, body string, hdr map[string]string) resp {
		t.Helper()
		return call(t, api, http.MethodPost, base+"/"+id+":transition", body, hdr)
	}
	states := func(body string) map[string]any {
		t.Helper()
		r := call(t, api, http.MethodPost, base+":field-states", body, nil)
		if r.code != 200 {
			t.Fatalf("field states: %d %s", r.code, r.raw)
		}
		return r.body
	}

	rec := call(t, api, http.MethodPost, base, `{"title": "WF2 Drucker", "kind": "fault"}`, nil)
	if rec.code != 201 {
		t.Fatalf("create: %d %s", rec.code, rec.raw)
	}
	id := rec.body["id"].(string)
	if r := transition(id, `{"transition": "start"}`, nil); r.code != 200 {
		t.Fatalf("start: %d %s", r.code, r.raw)
	}

	// Resolving without the solution: 422 naming the field.
	r := transition(id, `{"transition": "resolve"}`, nil)
	if r.code != 422 || !strings.Contains(r.raw, `"field":"solution"`) || !strings.Contains(r.raw, `"code":"required"`) {
		t.Fatalf("resolve without solution: %d %s", r.code, r.raw)
	}
	// With fields, If-Match is needed; with it, solution and state are one version.
	if r = transition(id, `{"transition": "resolve", "fields": {"solution": "Toner getauscht"}}`, nil); r.code != 428 {
		t.Fatalf("fields without If-Match: %d %s", r.code, r.raw)
	}
	r = transition(id, `{"transition": "resolve", "fields": {"solution": "Toner getauscht"}}`, map[string]string{"If-Match": `"2"`})
	if r.code != 200 || r.body["state"] != "resolved" || r.body["solution"] != "Toner getauscht" || r.body["version"] != float64(3) || r.hdr.Get("ETag") != `"3"` {
		t.Fatalf("resolve with solution: %d %s", r.code, r.raw)
	}

	// Closed: every field and the roles are read-only; the form is told so.
	if r = transition(id, `{"transition": "close"}`, map[string]string{"If-Match": `"3"`}); r.code != 200 || r.body["state"] != "closed" {
		t.Fatalf("close: %d %s", r.code, r.raw)
	}
	r = call(t, api, http.MethodPatch, base+"/"+id, `{"title": "geändert"}`, map[string]string{"If-Match": `"4"`})
	if r.code != 422 || !strings.Contains(r.raw, `"code":"read_only"`) {
		t.Fatalf("change a closed record: %d %s", r.code, r.raw)
	}
	fs := states(`{"id": "` + id + `"}`)
	if fs["state"] != "closed" || fs["locked"] != true {
		t.Fatalf("field states of a closed record: %v", fs)
	}
	ro, _ := fs["read_only"].([]any)
	if len(ro) < 5 {
		t.Fatalf("all fields read-only: %v", fs)
	}

	// A wish (a form variant by kind): impact and urgency hidden in the form, priority preset.
	fs = states(`{"values": {"kind": "wish"}}`)
	hidden, _ := fs["hidden"].([]any)
	defaults, _ := fs["defaults"].(map[string]any)
	if fs["state"] != "new" || len(hidden) != 3 || defaults["priority"] != float64(4) {
		t.Fatalf("draft wish: %v", fs)
	}
	wish := call(t, api, http.MethodPost, base, `{"title": "WF2 Wunsch", "kind": "wish"}`, nil)
	if wish.code != 201 || wish.body["priority"] != float64(4) {
		t.Fatalf("wish with preset: %d %s", wish.code, wish.raw)
	}

	// The event of the transition names the solution beside the state.
	r = call(t, api, http.MethodGet, base+"/"+id+"/history", "", nil)
	var hist []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &hist)
	v3 := map[string]string{}
	for _, h := range hist {
		if field, _ := h["field"].(string); h["version"] == float64(3) {
			v3[field], _ = h["action"].(string)
		}
	}
	if v3["state"] != "transition" || v3["solution"] != "update" {
		t.Fatalf("history of version 3: %v (%s)", v3, r.raw)
	}

	for _, rid := range []string{id, wish.body["id"].(string)} {
		call(t, api, http.MethodDelete, base+"/"+rid, "", nil)
		call(t, api, http.MethodPost, base+"/"+rid+":purge", "", nil)
	}
}
