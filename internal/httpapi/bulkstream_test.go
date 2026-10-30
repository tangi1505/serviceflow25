// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// bulkReport is the output of a bulk job as the test reads it.
type bulkReport struct {
	Done      bool `json:"done"`
	Lines     int  `json:"lines"`
	Created   int  `json:"created"`
	Updated   int  `json:"updated"`
	Unchanged int  `json:"unchanged"`
	Errors    int  `json:"errors"`
	Failures  []struct {
		Line        int    `json:"line"`
		ExternalKey string `json:"external_key"`
		Code        string `json:"code"`
		Message     string `json:"message"`
	} `json:"failures"`
}

// stream sends an NDJSON stream and returns status, headers and body.
func stream(t *testing.T, api *testAPI, source string, body []byte, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	h := map[string]string{"Content-Type": "application/x-ndjson"}
	for k, v := range hdr {
		h[k] = v
	}
	return raw(t, api, http.MethodPost, "/api/v1/records/probe.probe:bulk?source="+source, body, h)
}

// waitJob polls the job until its run is done and returns the report.
func waitJob(t *testing.T, api *testAPI, location string) bulkReport {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		code, _, body := raw(t, api, http.MethodGet, location, nil, nil)
		if code != 200 {
			t.Fatalf("job: %d %s", code, body)
		}
		var job struct {
			State  string          `json:"state"`
			Output json.RawMessage `json:"output"`
			Errors []any           `json:"errors"`
		}
		if err := json.Unmarshal(body, &job); err != nil {
			t.Fatal(err)
		}
		var rep bulkReport
		if len(job.Output) > 0 {
			if err := json.Unmarshal(job.Output, &rep); err != nil {
				t.Fatal(err)
			}
		}
		switch {
		case job.State == "completed" && rep.Done:
			return rep
		case job.State == "discarded" || job.State == "cancelled" || len(job.Errors) > 0:
			t.Fatalf("job %s: %s", job.State, body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the job did not finish in four minutes")
	return bulkReport{}
}

// The acceptance of PL32: 100,000 rows as NDJSON give 202 with the job; the job's output shows
// the progress until it is done, and the numbers are right.
func TestBulkStreamOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	source := fmt.Sprintf("apitest-stream-%d", time.Now().UnixNano())
	const n = 100000
	var buf bytes.Buffer
	for i := range n {
		fmt.Fprintf(&buf, `{"external_key": "dev-%06d", "title": "Gerät %d", "priority": %d}`+"\n", i, i, i%5+1)
	}
	start := time.Now()
	code, hdr, body := stream(t, api, source, buf.Bytes(), map[string]string{"Idempotency-Key": source})
	location := hdr.Get("Location")
	if code != 202 || !strings.HasPrefix(location, "/api/v1/jobs/") || !strings.Contains(string(body), `"kind":"bulk.apply"`) {
		t.Fatalf("stream: %d %q %s", code, location, body)
	}
	rep := waitJob(t, api, location)
	took := time.Since(start)
	t.Logf("%d rows created in %v (%.0f rows/s)", n, took.Round(time.Millisecond), n/took.Seconds())
	if rep.Lines != n || rep.Created != n || rep.Updated != 0 || rep.Unchanged != 0 || rep.Errors != 0 {
		t.Fatalf("first run: %+v", rep)
	}
	var rows, events, audits int
	counts := func() {
		t.Helper()
		if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM rec_probe_probe WHERE source = $1", source).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM event WHERE type_name = 'probe.probe' AND record_id IN (SELECT id FROM rec_probe_probe WHERE source = $1)", source).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE type_name = 'probe.probe' AND record_id IN (SELECT id FROM rec_probe_probe WHERE source = $1)", source).Scan(&audits); err != nil {
			t.Fatal(err)
		}
	}
	counts()
	// create plus title, priority and the defaults active and kind per record.
	if rows != n || events != n || audits != 5*n {
		t.Fatalf("written: %d rows, %d events, %d audit rows", rows, events, audits)
	}
	// The record is like one created over the API: values, state, search, history.
	var id string
	if err := api.pool.QueryRow(ctx, "SELECT id::text FROM rec_probe_probe WHERE source = $1 AND external_key = 'dev-004711'", source).Scan(&id); err != nil {
		t.Fatal(err)
	}
	rec := call(t, api, http.MethodGet, "/api/v1/records/probe.probe/"+id, "", nil)
	if rec.body["title"] != "Gerät 4711" || rec.body["priority"] != 2.0 || rec.body["state"] != "new" || rec.body["kind"] != "question" || rec.body["version"] != 1.0 {
		t.Fatalf("record: %s", rec.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/search?q=4711&types=probe.probe", "", nil); !strings.Contains(r.raw, id) {
		t.Fatalf("search does not find the record: %.300s", r.raw)
	}
	if r := call(t, api, http.MethodGet, "/api/v1/records/probe.probe/"+id+"/history", "", nil); !strings.Contains(r.raw, `"action":"create"`) || !strings.Contains(r.raw, "Gerät 4711") {
		t.Fatalf("history: %s", r.raw)
	}

	// The same stream again: all unchanged, nothing written.
	start = time.Now()
	code, hdr, body = stream(t, api, source, buf.Bytes(), nil)
	if code != 202 {
		t.Fatalf("second stream: %d %s", code, body)
	}
	rep = waitJob(t, api, hdr.Get("Location"))
	t.Logf("%d rows unchanged in %v", n, time.Since(start).Round(time.Millisecond))
	if rep.Unchanged != n || rep.Created != 0 || rep.Updated != 0 {
		t.Fatalf("second run: %+v", rep)
	}
	before := events
	counts()
	if events != before {
		t.Fatalf("an unchanged run wrote %d events", events-before)
	}

	// The same key and stream: the same answer, no second job.
	code2, hdr2, body2 := stream(t, api, source, buf.Bytes(), map[string]string{"Idempotency-Key": source})
	if code2 != 202 || hdr2.Get("Location") != location || hdr2.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay: %d %q %s", code2, hdr2.Get("Location"), body2)
	}

	// Rows fail on their own and are listed with their line.
	mixed := strings.Join([]string{
		`{"external_key": "dev-000001", "title": "Gerät 1 neu", "priority": 2}`,
		`not json`,
		`{"title": "ohne Schlüssel"}`,
		``,
		`{"external_key": "new-1", "title": "neu"}`,
		`{"external_key": "new-1", "title": "doppelt"}`,
		`{"external_key": "new-2", "title": "zu hoch", "priority": 9}`,
	}, "\n")
	code, hdr, body = stream(t, api, source, []byte(mixed), nil)
	if code != 202 {
		t.Fatalf("mixed: %d %s", code, body)
	}
	rep = waitJob(t, api, hdr.Get("Location"))
	if rep.Lines != 6 || rep.Updated != 1 || rep.Created != 1 || rep.Errors != 4 || len(rep.Failures) != 4 {
		t.Fatalf("mixed run: %+v", rep)
	}
	lines := map[int]string{}
	for _, f := range rep.Failures {
		lines[f.Line] = f.Code
	}
	if lines[2] != "validation" || lines[3] != "validation" || lines[6] != "conflict" || lines[7] != "validation" {
		t.Fatalf("failures by line: %v", lines)
	}

	// Calls that cannot run.
	if code, _, body := stream(t, api, "", []byte(`{"external_key": "a"}`), nil); code != 422 || !strings.Contains(string(body), `"field":"source"`) {
		t.Fatalf("no source: %d %s", code, body)
	}
	if code, _, body := stream(t, api, source, nil, nil); code != 422 || !strings.Contains(string(body), "empty") {
		t.Fatalf("empty stream: %d %s", code, body)
	}
	set := call(t, api, http.MethodPut, "/api/v1/org/nodes/root/settings/bulk.max_size", "100", nil)
	if set.code != 204 {
		t.Fatalf("setting: %d %s", set.code, set.raw)
	}
	t.Cleanup(func() {
		_, _ = api.pool.Exec(context.Background(), "DELETE FROM org_setting WHERE path = 'root' AND key = 'bulk.max_size'")
	})
	if code, _, body := stream(t, api, source, buf.Bytes()[:1000], nil); code != 413 {
		t.Fatalf("too large: %d %s", code, body)
	}
}

// Whoever started a bulk run may follow its job without rights on core.job, but no other job
// (PL32); the rows of an account without rights on the type fail with forbidden.
func TestBulkStreamOwnJob(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	acc, err := admin.auth.CreateAccount(ctx, "apitest_bulkown", "apitest_bulkown", testPassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := call(t, admin, http.MethodPost, "/api/v1/records/core.person", `{"last_name": "apitest_bulkown"}`, nil)
	pid := uuid.MustParse(p.body["id"].(string))
	if err := admin.authz.LinkPerson(ctx, acc.ID, &pid); err != nil {
		t.Fatal(err)
	}
	login := call(t, admin, http.MethodPost, "/api/v1/auth/login", `{"username": "apitest_bulkown", "password": "`+testPassword+`"}`, map[string]string{"Cookie": ""})
	user := &testAPI{Server: admin.Server, cookie: sessionCookieOf(t, login.hdr), auth: admin.auth, authz: admin.authz, pool: admin.pool, kv: admin.kv}

	source := fmt.Sprintf("apitest-own-%d", time.Now().UnixNano())
	code, hdr, body := stream(t, user, source, []byte(`{"external_key": "a", "title": "A"}`+"\n"+`{"external_key": "b", "title": "B"}`+"\n"), nil)
	if code != 202 {
		t.Fatalf("stream: %d %s", code, body)
	}
	rep := waitJob(t, user, hdr.Get("Location"))
	if rep.Errors != 2 || rep.Created != 0 || len(rep.Failures) != 2 || rep.Failures[0].Code != "forbidden" {
		t.Fatalf("rows without rights: %+v", rep)
	}

	// A job of the administrator stays closed to the account.
	code, hdr, _ = stream(t, admin, source, []byte(`{"external_key": "c", "title": "C"}`+"\n"), nil)
	if code != 202 {
		t.Fatalf("admin stream: %d", code)
	}
	if code, _, body := raw(t, user, http.MethodGet, hdr.Get("Location"), nil, nil); code != 403 {
		t.Fatalf("someone else's job: %d %s", code, body)
	}
	if code, _, _ := raw(t, user, http.MethodGet, "/api/v1/jobs", nil, nil); code != 403 {
		t.Fatalf("the job list: %d", code)
	}
	waitJob(t, admin, hdr.Get("Location"))
}
