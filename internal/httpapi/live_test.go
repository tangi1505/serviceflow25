// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// UI8 over HTTP: who has a record open sees the others, the one editing first; whoever may
// not read the record learns nothing; leaving takes one off. The stream marks the events of
// records where the person holds a role as mine.
func TestLiveOverHTTP(t *testing.T) {
	admin := apiServer(t)
	timelineRoles(t, admin, "ui8")
	anna := timelineLogin(t, admin, "apitest_ui8_anna", "ui8_agent", `{"last_name": "UI8 Anna"}`)
	ben := timelineLogin(t, admin, "apitest_ui8_ben", "ui8_reader", `{"last_name": "UI8 Ben"}`)
	rec := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "UI8 Router", "actors": {"requester": [{"type": "core.person", "id": "`+anna.person+`"}]}}`, nil)
	other := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "UI8 Drucker"}`, nil)
	base := "/api/v1/records/probe.probe/" + rec.body["id"].(string)

	beat := func(u *testAPI, editing bool) []map[string]any {
		t.Helper()
		r := call(t, u, http.MethodPut, base+"/presence", fmt.Sprintf(`{"editing": %t}`, editing), nil)
		if r.code != 200 {
			t.Fatalf("beat: %d %s", r.code, r.raw)
		}
		var out struct {
			Others []map[string]any `json:"others"`
		}
		_ = json.Unmarshal([]byte(r.raw), &out)
		return out.Others
	}
	call(t, admin, http.MethodDelete, base+"/presence", "", nil)
	call(t, anna.api, http.MethodDelete, base+"/presence", "", nil)
	call(t, ben.api, http.MethodDelete, base+"/presence", "", nil)
	if others := beat(anna.api, false); len(others) != 0 {
		t.Fatalf("Anna alone: %v", others)
	}
	beat(ben.api, true)
	others := beat(admin, false)
	if len(others) != 2 || others[0]["name"] != "apitest_ui8_ben" || others[0]["editing"] != true || others[1]["name"] != "apitest_ui8_anna" {
		t.Fatalf("admin sees %v", others)
	}
	if r := call(t, ben.api, http.MethodDelete, base+"/presence", "", nil); r.code != 204 {
		t.Fatalf("leave: %d", r.code)
	}
	if others := beat(anna.api, true); len(others) != 1 || others[0]["name"] != "API Test" {
		t.Fatalf("after Ben left: %v", others)
	}
	// A record one may not read: no presence there.
	call(t, admin, http.MethodDelete, "/api/v1/roles/ui8_none", "", nil)
	call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "ui8_none", "label": {"de": "nichts"}}`, nil)
	nobody := timelineLogin(t, admin, "apitest_ui8_nobody", "ui8_none", `{"last_name": "UI8 Niemand"}`)
	if r := call(t, nobody.api, http.MethodPut, base+"/presence", `{"editing": false}`, nil); r.code != 404 {
		t.Fatalf("presence without read: %d %s", r.code, r.raw)
	}

	// The stream: Anna holds the requester role on the first record, not on the second.
	pos := call(t, anna.api, http.MethodGet, "/api/v1/events/position", "", nil).body["position"].(float64)
	for _, id := range []string{rec.body["id"].(string), other.body["id"].(string)} {
		r := call(t, admin, http.MethodPatch, "/api/v1/records/probe.probe/"+id, `{"priority": 2}`, map[string]string{"If-Match": `"1"`})
		if r.code != 200 {
			t.Fatalf("change: %d %s", r.code, r.raw)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, anna.api.URL+fmt.Sprintf("/api/v1/events/stream?after=%d", int64(pos)), nil)
	req.Header.Set("Cookie", sessionCookie+"="+anna.api.cookie)
	res, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed below, after the stream was read
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	mine := map[string]bool{}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() && len(mine) < 2 {
		line, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var e struct {
			Kind     string `json:"kind"`
			RecordID string `json:"record_id"`
			Mine     bool   `json:"mine"`
		}
		if json.Unmarshal([]byte(line), &e) == nil && e.Kind == "update" {
			mine[e.RecordID] = e.Mine
		}
	}
	if !mine[rec.body["id"].(string)] {
		t.Fatalf("the event of Anna's record is not mine: %v", mine)
	}
	if v, ok := mine[other.body["id"].(string)]; !ok || v {
		t.Fatalf("the event of the other record: %v", mine)
	}
}
