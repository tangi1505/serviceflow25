// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The Prüfweg of PL76 with a fixed clock: at the default of 30 days a record is still in the
// trash after 29 days and gone after 31; at a unit with 7 days it is gone after 8; at a unit
// with 0 it stays; the history names the system (no actor) and the event stream the purge.
func TestTrashEmptiesItself(t *testing.T) {
	admin := apiServer(t)
	ctx := t.Context()
	tag := "PL76 " + uuid.NewString()[:8]
	day := 24 * time.Hour
	for _, unit := range []string{"pl76week", "pl76never"} {
		if r := call(t, admin, http.MethodPost, "/api/v1/org/nodes", `{"parent": "root", "name": "`+unit+`"}`, nil); r.code != 201 && r.code != 409 {
			t.Fatalf("unit %s: %d %s", unit, r.code, r.raw)
		}
	}
	for path, days := range map[string]string{"root.pl76week": "7", "root.pl76never": "0"} {
		if r := call(t, admin, http.MethodPut, "/api/v1/org/nodes/"+path+"/settings/trash.keep_days", days, nil); r.code != 204 {
			t.Fatalf("setting at %s: %d %s", path, r.code, r.raw)
		}
	}
	admin.settings.Forget()

	ids := map[string]string{}
	for name, path := range map[string]string{"root": "root", "week": "root.pl76week", "never": "root.pl76never"} {
		r := call(t, admin, http.MethodPost, "/api/v1/records/probe.probe", `{"title": "`+tag+` `+name+`", "org_path": "`+path+`"}`, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", name, r.code, r.raw)
		}
		ids[name] = r.body["id"].(string)
		if r := call(t, admin, http.MethodDelete, "/api/v1/records/probe.probe/"+ids[name], "", nil); r.code != 204 {
			t.Fatalf("trash %s: %d %s", name, r.code, r.raw)
		}
	}
	defer func() {
		call(t, admin, http.MethodPost, "/api/v1/records/probe.probe/"+ids["never"]+":purge", "", nil)
	}()
	inTrash := func() map[string]bool {
		t.Helper()
		r := call(t, admin, http.MethodGet, "/api/v1/trash?q="+url.QueryEscape(tag), "", nil)
		if r.code != 200 {
			t.Fatalf("trash: %d %s", r.code, r.raw)
		}
		out := map[string]bool{}
		for _, it := range r.body["items"].([]any) {
			for name, id := range ids {
				if it.(map[string]any)["id"] == id {
					out[name] = true
				}
			}
		}
		return out
	}
	empty := func(after time.Duration) {
		t.Helper()
		if _, err := admin.records.EmptyTrash(ctx, time.Now().Add(after)); err != nil {
			t.Fatalf("empty after %s: %v", after, err)
		}
	}

	empty(6 * day)
	if got := inTrash(); !got["root"] || !got["week"] || !got["never"] {
		t.Fatalf("after 6 days: %v", got)
	}
	empty(8 * day)
	if got := inTrash(); !got["root"] || got["week"] || !got["never"] {
		t.Fatalf("after 8 days the unit with 7 is empty: %v", got)
	}
	empty(29 * day)
	if got := inTrash(); !got["root"] || !got["never"] {
		t.Fatalf("after 29 days: %v", got)
	}
	empty(31 * day)
	if got := inTrash(); got["root"] || !got["never"] {
		t.Fatalf("after 31 days only the unit with 0 keeps its record: %v", got)
	}
	empty(4000 * day)
	if got := inTrash(); !got["never"] {
		t.Fatalf("0 means never: %v", got)
	}

	// The purge is in the history as the system's (no actor) and on the event stream.
	for _, name := range []string{"root", "week"} {
		var actor *uuid.UUID
		if err := admin.pool.QueryRow(ctx, `SELECT actor FROM audit WHERE type_name = 'probe.probe' AND record_id = $1 AND action = 'purge'`, uuid.MustParse(ids[name])).Scan(&actor); err != nil || actor != nil {
			t.Fatalf("audit of %s: %v %v", name, actor, err)
		}
		var n int
		if err := admin.pool.QueryRow(ctx, `SELECT count(*) FROM event WHERE type_name = 'probe.probe' AND record_id = $1 AND kind = 'purge'`, uuid.MustParse(ids[name])).Scan(&n); err != nil || n != 1 {
			t.Fatalf("event of %s: %d %v", name, n, err)
		}
	}
}
