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
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/events"
)

// The acceptance of PL24 over HTTP: ten changes are ten events in order, a subscriber at
// five catches up with six to ten, a position too old is 410, rights hide foreign events,
// and SSE replays then streams live.
func TestEventsAPI(t *testing.T) {
	// A database of its own: the retention below deletes the start of the event log, which
	// the tests of other packages running at the same time still read.
	if base := os.Getenv("ITSM_TEST_DATABASE_URL"); base != "" {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_events"))
	}
	admin := apiServer(t)
	ctx := t.Context()
	const base = "/api/v1/records/probe.probe"

	r := call(t, admin, http.MethodGet, "/api/v1/events/position", "", nil)
	if r.code != 200 {
		t.Fatalf("position: %d %s", r.code, r.raw)
	}
	start := int64(r.body["position"].(float64))

	var ids []string
	for i := range 10 {
		r := call(t, admin, http.MethodPost, base, fmt.Sprintf(`{"title": "event %d"}`, i), nil)
		if r.code != 201 {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
		ids = append(ids, r.body["id"].(string))
	}
	list := func(api *testAPI, after int64) ([]map[string]any, resp) {
		t.Helper()
		r := call(t, api, http.MethodGet, fmt.Sprintf("/api/v1/events?after=%d&limit=1000", after), "", nil)
		var out []map[string]any
		_ = json.Unmarshal([]byte(r.raw), &out)
		return out, r
	}
	// Other packages test against the same database at the same time and the admin sees
	// every event, so the checks look at the events of these ten records.
	mineOf := func(list []map[string]any) []map[string]any {
		var out []map[string]any
		for _, e := range list {
			for _, id := range ids {
				if e["record_id"] == id {
					out = append(out, e)
				}
			}
		}
		return out
	}
	// An open write transaction of another test package holds the stream back for a moment
	// (the horizon, E87): read until the ten events are visible.
	all, r := list(admin, start)
	mine := mineOf(all)
	for deadline := time.Now().Add(10 * time.Second); r.code == 200 && len(mine) < 10 && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
		all, r = list(admin, start)
		mine = mineOf(all)
	}
	if r.code != 200 || len(mine) != 10 {
		t.Fatalf("ten events: %d %d %s", r.code, len(mine), r.raw)
	}
	pos := make([]int64, 10)
	for i, e := range mine {
		pos[i] = int64(e["position"].(float64))
		if e["kind"] != "create" || e["record_id"] != ids[i] || (i > 0 && pos[i] <= pos[i-1]) {
			t.Fatalf("event %d out of order: %v", i, e)
		}
	}
	if last, _ := strconv.ParseInt(r.hdr.Get("ITSM-Last-Position"), 10, 64); last < pos[9] {
		t.Fatalf("last position %d must reach the tenth event %d", last, pos[9])
	}
	tail, r := list(admin, pos[4])
	if tail = mineOf(tail); r.code != 200 || len(tail) != 5 || tail[0]["record_id"] != ids[5] || tail[4]["record_id"] != ids[9] {
		t.Fatalf("catch up from the fifth: %d %s", r.code, r.raw)
	}

	// A person who reads only own records sees only own events, the positions still advance.
	dropRole(t, admin, "ev_own")
	if r := call(t, admin, http.MethodPost, "/api/v1/roles", `{"name": "ev_own"}`, nil); r.code != 201 {
		t.Fatalf("role: %d %s", r.code, r.raw)
	}
	for _, action := range []string{"read", "create", "delete"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/roles/ev_own/permissions", `{"type_name": "probe.probe", "action": "`+action+`", "scope": "own"}`, nil); r.code != 204 {
			t.Fatalf("permission: %d %s", r.code, r.raw)
		}
	}
	acc, err := admin.auth.CreateAccount(ctx, "apitest_ev", "apitest_ev", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "apitest_ev"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	if r := call(t, admin, http.MethodPost, "/api/v1/assignments", `{"role": "ev_own", "person_id": "`+pid.String()+`", "org_path": "root"}`, nil); r.code != 201 {
		t.Fatalf("assign: %d %s", r.code, r.raw)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_ev", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	own := call(t, user, http.MethodPost, base, `{"title": "mine"}`, nil)
	if own.code != 201 {
		t.Fatalf("own record: %d %s", own.code, own.raw)
	}
	ownList, r := list(user, start)
	if last, _ := strconv.ParseInt(r.hdr.Get("ITSM-Last-Position"), 10, 64); r.code != 200 || len(ownList) != 1 || ownList[0]["record_id"] != own.body["id"] || last <= pos[9] {
		t.Fatalf("own events only, positions advance past hidden ones: %d %d %s %s", r.code, len(ownList), r.hdr.Get("ITSM-Last-Position"), r.raw)
	}
	// After a purge the payload decides: the own record stays visible, foreign ones not.
	call(t, user, http.MethodDelete, base+"/"+own.body["id"].(string), "", nil)
	call(t, user, http.MethodPost, base+"/"+own.body["id"].(string)+":purge", "", nil)
	call(t, admin, http.MethodDelete, base+"/"+ids[0], "", nil)
	call(t, admin, http.MethodPost, base+"/"+ids[0]+":purge", "", nil)
	ownList, r = list(user, pos[9])
	if r.code != 200 || len(ownList) != 3 || ownList[2]["kind"] != "purge" {
		t.Fatalf("own events after purge: %d %s", r.code, r.raw)
	}

	// SSE: replay from a position, then live through the dispatcher. The test's dispatcher
	// starts at the end of the log under a name of its own: the shared dispatcher may still
	// work through the backlog of other tests (100,000 events of the bulk, PL32).
	admin.events.Prefix = "apitest-events-"
	if err := admin.events.SkipToEnd(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.pool.Exec(context.Background(), "DELETE FROM event_consumer WHERE name LIKE 'apitest-events-%'")
	})
	go func() { _ = admin.events.RunDispatcher(ctx) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, admin.URL+fmt.Sprintf("/api/v1/events/stream?after=%d", pos[7]), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", sessionCookie+"="+admin.cookie)
	res, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed below, after the stream was read
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	lines := make(chan string, 256)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	var seen []string
	waitLine := func(prefix string) string {
		t.Helper()
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("stream closed; seen %v", seen)
				}
				seen = append(seen, l)
				if strings.HasPrefix(l, prefix) {
					return l
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("no line %q; seen %v", prefix, seen)
			}
		}
	}
	// The replay: the ninth and tenth event, then the later ones, then connected.
	waitLine(fmt.Sprintf("id: %d", pos[8]))
	waitLine(fmt.Sprintf("id: %d", pos[9]))
	waitLine(": connected")
	live := call(t, admin, http.MethodPost, base, `{"title": "live"}`, nil)
	if live.code != 201 {
		t.Fatalf("live record: %d %s", live.code, live.raw)
	}
	data := waitLine("data: ")
	var e map[string]any
	_ = json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &e)
	if e["record_id"] != live.body["id"] || e["kind"] != "create" {
		t.Fatalf("live event: %s", data)
	}
	// Retention: the oldest events gone, a position before them is 410.
	if _, err := admin.pool.Exec(ctx, "DELETE FROM event WHERE position <= $1", pos[4]); err != nil {
		t.Fatal(err)
	}
	if _, r := list(admin, pos[4]-2); r.code != 410 {
		t.Fatalf("too old: %d %s", r.code, r.raw)
	}
	if _, r := list(admin, pos[4]); r.code != 200 {
		t.Fatalf("at the oldest kept: %d %s", r.code, r.raw)
	}
	_ = events.Channel
}
