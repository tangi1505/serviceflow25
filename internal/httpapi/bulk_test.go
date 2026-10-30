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
	"testing"
	"time"
)

// The acceptance of PL31: 1,000 rows once are 1,000 created; the same again are 1,000
// unchanged, without history and without events; the same Idempotency-Key gets the same
// answer.
func TestBulkOverHTTP(t *testing.T) {
	api := apiServer(t)
	ctx := t.Context()
	source := fmt.Sprintf("apitest-%d", time.Now().UnixNano())
	const path = "/api/v1/records/probe.probe:bulk"
	rows := make([]map[string]any, 1000)
	for i := range rows {
		rows[i] = map[string]any{"external_key": fmt.Sprintf("vm-%04d", i), "title": fmt.Sprintf("VM %d", i), "priority": 2}
	}
	body := func(rows []map[string]any) string {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"source": source, "rows": rows})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	counts := func(r resp) (created, updated, unchanged, errs float64) {
		t.Helper()
		if r.code != 200 {
			t.Fatalf("bulk: %d %.500s", r.code, r.raw)
		}
		return r.body["created"].(float64), r.body["updated"].(float64), r.body["unchanged"].(float64), r.body["errors"].(float64)
	}
	item := func(r resp, i int) map[string]any {
		return r.body["items"].([]any)[i].(map[string]any)
	}

	start := time.Now()
	first := call(t, api, http.MethodPost, path, body(rows), map[string]string{"Idempotency-Key": source + "-1"})
	if c, u, n, e := counts(first); c != 1000 || u != 0 || n != 0 || e != 0 {
		t.Fatalf("first run: %v %v %v %v", c, u, n, e)
	}
	t.Logf("1,000 rows created in %v", time.Since(start).Round(time.Millisecond))
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = item(first, i)["id"].(string)
	}
	written := func() (audits, events int) {
		t.Helper()
		if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE type_name = 'probe.probe' AND record_id::text = ANY($1)", ids).Scan(&audits); err != nil {
			t.Fatal(err)
		}
		if err := api.pool.QueryRow(ctx, "SELECT count(*) FROM event WHERE type_name = 'probe.probe' AND record_id::text = ANY($1)", ids).Scan(&events); err != nil {
			t.Fatal(err)
		}
		return audits, events
	}
	audits, events := written()
	if events != 1000 {
		t.Fatalf("events of the first run: %d", events)
	}

	// The same rows again, without a key: nothing to write.
	start = time.Now()
	second := call(t, api, http.MethodPost, path, body(rows), nil)
	if c, u, n, e := counts(second); c != 0 || u != 0 || n != 1000 || e != 0 {
		t.Fatalf("second run: %v %v %v %v", c, u, n, e)
	}
	t.Logf("1,000 rows unchanged in %v", time.Since(start).Round(time.Millisecond))
	if a, e := written(); a != audits || e != events {
		t.Fatalf("an unchanged run wrote history or events: audit %d → %d, events %d → %d", audits, a, events, e)
	}
	if v := item(second, 0)["version"].(float64); v != 1 || item(second, 0)["id"] != ids[0] {
		t.Fatalf("unchanged row: %v", item(second, 0))
	}

	// The same key with the same body: the same answer, nothing run again; with another body 422.
	replay := call(t, api, http.MethodPost, path, body(rows), map[string]string{"Idempotency-Key": source + "-1"})
	if replay.code != 200 || replay.raw != first.raw || replay.hdr.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay: %d %q %.200s", replay.code, replay.hdr.Get("Idempotent-Replayed"), replay.raw)
	}
	if r := call(t, api, http.MethodPost, path, body(rows[:10]), map[string]string{"Idempotency-Key": source + "-1"}); r.code != 422 || !strings.Contains(r.raw, "reused") {
		t.Fatalf("key with another body: %d %s", r.code, r.raw)
	}

	// A changed field is an update; a row with fewer fields leaves the others alone.
	changed := []map[string]any{
		{"external_key": "vm-0000", "title": "VM 0 renamed", "priority": 2},
		{"external_key": "vm-0001", "title": "VM 1"},
	}
	r := call(t, api, http.MethodPost, path, body(changed), nil)
	if c, u, n, e := counts(r); c != 0 || u != 1 || n != 1 || e != 0 {
		t.Fatalf("change: %v %v %v %v %s", c, u, n, e, r.raw)
	}
	if v := item(r, 0)["version"].(float64); v != 2 {
		t.Fatalf("updated row: %v", item(r, 0))
	}
	if got := call(t, api, http.MethodGet, "/api/v1/records/probe.probe/"+ids[1], "", nil); got.body["priority"] != 2.0 || got.body["source"] != source || got.body["external_key"] != "vm-0001" {
		t.Fatalf("field not delivered: %s", got.raw)
	}

	// Rows fail on their own; the others are written.
	mixed := []map[string]any{
		{"title": "without key"},
		{"external_key": "vm-new", "title": "new"},
		{"external_key": "vm-bad", "title": "bad", "priority": 9},
	}
	r = call(t, api, http.MethodPost, path, body(mixed), nil)
	if c, _, _, e := counts(r); c != 1 || e != 2 {
		t.Fatalf("mixed: %s", r.raw)
	}
	if bad := item(r, 2); bad["status"] != "error" || bad["code"] != "validation" || !strings.Contains(fmt.Sprint(bad["problems"]), "priority") {
		t.Fatalf("invalid row: %v", bad)
	}

	// A record in the trash is a conflict of its row.
	if r := call(t, api, http.MethodDelete, "/api/v1/records/probe.probe/"+ids[2], "", nil); r.code != 204 {
		t.Fatal(r.raw)
	}
	r = call(t, api, http.MethodPost, path, body([]map[string]any{{"external_key": "vm-0002", "title": "back?"}}), nil)
	if it := item(r, 0); it["code"] != "conflict" || !strings.Contains(fmt.Sprint(it["message"]), "trash") {
		t.Fatalf("trashed: %v", it)
	}

	// Calls that cannot run.
	if r := call(t, api, http.MethodPost, path, body(make([]map[string]any, 1001)), nil); r.code != 422 || !strings.Contains(r.raw, "at most 1000") {
		t.Fatalf("1,001 rows: %d %.300s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodPost, path, `{"source": "", "rows": [{"external_key": "a"}]}`, nil); r.code != 422 || !strings.Contains(r.raw, `"field":"source"`) {
		t.Fatalf("no source: %d %s", r.code, r.raw)
	}
	if r := call(t, api, http.MethodPost, "/api/v1/records/probe.nothing:bulk", body(rows[:1]), nil); r.code != 404 {
		t.Fatalf("unknown type: %d %s", r.code, r.raw)
	}

	// The effective description has the path per type, with the bulk body, not the record.
	spec := call(t, api, http.MethodGet, "/api/v1/openapi.json", "", nil)
	paths := spec.body["paths"].(map[string]any)
	op, _ := paths["/api/v1/records/probe.probe:bulk"].(map[string]any)
	if op == nil || !strings.Contains(fmt.Sprint(op["post"]), "BulkRequest") {
		t.Fatalf("bulk path in the description: %v", op)
	}
	if q, _ := paths["/api/v1/records/probe.probe:query"].(map[string]any); !strings.Contains(fmt.Sprint(q["post"]), "schemas/Query") {
		t.Fatalf("the query body became the record: %v", q)
	}
}
