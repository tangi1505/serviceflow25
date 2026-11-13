// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"group24.de/itsm/internal/meta"
)

// The Prüfweg of PL53 (found with H14): the types are kept in memory. A field the type editor
// adds is there at once; a change past this process, straight in the table definition as the
// command line or another instance makes it, arrives by the notice of the database within
// moments.
func TestTypesFollowDefinitionsOverHTTP(t *testing.T) {
	admin := apiServer(t)
	drop := func() {
		if _, err := admin.pool.Exec(context.Background(), "DELETE FROM definition WHERE package = 'probe' AND name = 'probe' AND layer = 'customer'"); err != nil {
			t.Fatal(err)
		}
		meta.DefinitionsChanged()
	}
	drop()
	t.Cleanup(drop)
	has := func() bool {
		r := call(t, admin, http.MethodGet, "/api/v1/types/probe.probe", "", nil)
		if r.code != 200 {
			t.Fatalf("type: %d %s", r.code, r.raw)
		}
		return strings.Contains(r.raw, `"k_pl53_cache"`)
	}
	r := call(t, admin, http.MethodPost, "/api/v1/types/probe.probe/customer",
		`{"version": 0, "change": {"op": "add_field", "field": "k_pl53_cache", "new": {"name": "k_pl53_cache", "type": "text", "label": {"de": "Im Speicher"}, "max_length": 40}}}`, nil)
	if r.code != 200 {
		t.Fatalf("add_field: %d %s", r.code, r.raw)
	}
	if !has() {
		t.Fatal("the new field is not there at once")
	}
	// Past this process: only the notice of the database tells the types in memory.
	if _, err := admin.pool.Exec(context.Background(), "DELETE FROM definition WHERE package = 'probe' AND name = 'probe' AND layer = 'customer'"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for has() {
		if time.Now().After(deadline) {
			t.Fatal("the change past this process did not arrive in 5 s")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
