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
	"testing"
)

// The API of PL23: schedules with override and run now, the log with filters, the rights.
func TestJobsAPI(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()

	r := call(t, admin, http.MethodGet, "/api/v1/jobs/schedules", "", nil)
	if r.code != 200 {
		t.Fatalf("schedules: %d %s", r.code, r.raw)
	}
	var schedules []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &schedules)
	var tick map[string]any
	for _, s := range schedules {
		if s["name"] == "apitest.tick" {
			tick = s
		}
	}
	if tick == nil || tick["enabled"] != true || tick["schedule"] != "*/10 * * * *" || tick["queue"] != "apitest" || tick["next_run"] == nil {
		t.Fatalf("apitest.tick in the schedules: %s", r.raw)
	}

	r = call(t, admin, http.MethodPatch, "/api/v1/jobs/schedules/apitest.tick", `{"enabled": false, "schedule": "0 4 * * *"}`, nil)
	if r.code != 200 || r.body["enabled"] != false || r.body["schedule"] != "0 4 * * *" || r.body["default_schedule"] != "*/10 * * * *" || r.body["next_run"] != nil {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/jobs/schedules/apitest.tick", "", nil); r.code != 200 || r.body["enabled"] != false {
		t.Fatalf("get after patch: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPatch, "/api/v1/jobs/schedules/apitest.tick", `{"schedule": "nope"}`, nil); r.code != 422 {
		t.Fatalf("bad schedule: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/jobs/schedules/apitest.none", "", nil); r.code != 404 {
		t.Fatalf("unknown schedule: %d %s", r.code, r.raw)
	}

	// Run now: the job appears in the log; no worker runs in this test, so it stays available.
	r = call(t, admin, http.MethodPost, "/api/v1/jobs/schedules/apitest.tick:run", "", nil)
	if r.code != 202 || r.body["kind"] != "apitest.tick" || r.body["state"] != "available" {
		t.Fatalf("run: %d %s", r.code, r.raw)
	}
	id := int64(r.body["id"].(float64))
	r = call(t, admin, http.MethodGet, "/api/v1/jobs?kind=apitest.tick&queue=apitest&state=available", "", nil)
	var list []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &list)
	if r.code != 200 || len(list) == 0 || int64(list[0]["id"].(float64)) != id {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/jobs?state=sleeping", "", nil); r.code != 422 {
		t.Fatalf("unknown state must be refused: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, fmt.Sprintf("/api/v1/jobs/%d", id), "", nil); r.code != 200 || r.body["kind"] != "apitest.tick" {
		t.Fatalf("get job: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d:cancel", id), "", nil); r.code != 200 || r.body["state"] != "cancelled" {
		t.Fatalf("cancel: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d:retry", id), "", nil); r.code != 200 || r.body["state"] != "available" {
		t.Fatalf("retry: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/jobs/4611686018427387904", "", nil); r.code != 404 {
		t.Fatalf("unknown job: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodGet, "/api/v1/jobs/x", "", nil); r.code != 400 {
		t.Fatalf("job id that is no number: %d %s", r.code, r.raw)
	}

	// A person without rights on core.job sees nothing and changes nothing.
	if _, err := admin.auth.CreateAccount(ctx, "apitest_nojobs", "apitest_nojobs", testPassword, nil); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_nojobs", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	if login.code != 200 {
		t.Fatalf("login: %d %s", login.code, login.raw)
	}
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr)}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/jobs"},
		{http.MethodGet, "/api/v1/jobs/schedules"},
		{http.MethodPatch, "/api/v1/jobs/schedules/apitest.tick"},
		{http.MethodPost, "/api/v1/jobs/schedules/apitest.tick:run"},
		{http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d:cancel", id)},
	} {
		body := ""
		if c.method == http.MethodPatch {
			body = `{"enabled": true}`
		}
		if r := call(t, user, c.method, c.path, body, nil); r.code != 403 {
			t.Fatalf("%s %s without rights: %d %s", c.method, c.path, r.code, r.raw)
		}
	}
}
