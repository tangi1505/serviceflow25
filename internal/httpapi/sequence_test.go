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
	"net/url"
	"os"
	"sync"
	"testing"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/tenant"
)

// The Prüfweg of PL49 (Kern-Plan E63) in both ways of keeping tenants: the running number
// starts at 1 in every tenant and goes on; 100 records made at once get 100 numbers; PATCH
// cannot set it; the search finds it with and without the prefix; an import with a source may
// bring an old number, the sequence goes on above it, and the same row again keeps it.
func TestSequenceOverHTTP(t *testing.T) {
	t.Run("shared", func(t *testing.T) { sequenceCase(t, false) })
	t.Run("database", func(t *testing.T) { sequenceCase(t, true) })
}

func sequenceCase(t *testing.T, separate bool) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx := t.Context()
	var admin *testAPI
	if separate {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl49_ctl"))
		urls := map[string]string{"root.ta": dbtest.Isolated(t, base, "itsmtest_api_pl49_ta"), "root.tb": dbtest.Isolated(t, base, "itsmtest_api_pl49_tb")}
		admin = apiServerWith(t, urls)
		tenantsInDatabases(t, admin, urls)
	} else {
		t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl49"))
		admin = apiServer(t)
		for _, name := range []string{"ta", "tb"} {
			if _, err := tenant.Create(ctx, admin.pool, tenant.Options{Name: name}); err != nil {
				t.Fatal(err)
			}
		}
		admin.authz.Invalidate(ctx)
	}
	const base0 = "/api/v1/records/probe.probe"
	enter := func(path string) {
		t.Helper()
		if r := call(t, admin, http.MethodPost, "/api/v1/auth/context", `{"tenant": "`+path+`"}`, nil); r.code != 200 {
			t.Fatalf("switch to %s: %d %s", path, r.code, r.raw)
		}
	}
	create := func(title string) map[string]any {
		t.Helper()
		r := call(t, admin, http.MethodPost, base0, `{"title": "`+title+`"}`, nil)
		if r.code != 201 {
			t.Fatalf("create %s: %d %s", title, r.code, r.raw)
		}
		return r.body
	}

	// A starts at 1 and goes on; B starts at 1 again.
	enter("root.ta")
	a1, a2 := create("PL49 A eins"), create("PL49 A zwei")
	if a1["number"] != 1.0 || a2["number"] != 2.0 {
		t.Fatalf("numbers in A: %v %v", a1["number"], a2["number"])
	}
	enter("root.tb")
	if b1 := create("PL49 B eins"); b1["number"] != 1.0 {
		t.Fatalf("number in B: %v", b1["number"])
	}

	// 100 records at once: 100 different numbers. One request first, so that the mirror of the
	// administrator in A is up to date before the 100 come at once (PL47b).
	enter("root.ta")
	if a3 := create("PL49 A drei"); a3["number"] != 3.0 {
		t.Fatalf("third in A: %v", a3["number"])
	}
	var mu sync.Mutex
	seen := map[float64]bool{}
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := call(t, admin, http.MethodPost, base0, fmt.Sprintf(`{"title": "PL49 parallel %d"}`, i), nil)
			if r.code != 201 {
				t.Errorf("parallel %d: %d %s", i, r.code, r.raw)
				return
			}
			mu.Lock()
			seen[r.body["number"].(float64)] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != 100 || seen[1] || seen[2] || seen[3] {
		t.Fatalf("100 at once gave %d numbers: %v", len(seen), seen)
	}

	// PATCH and create cannot set it.
	id2 := a2["id"].(string)
	if r := call(t, admin, http.MethodPatch, base0+"/"+id2, `{"number": 7}`, map[string]string{"If-Match": `"1"`}); r.code != 422 {
		t.Fatalf("PATCH of the number: %d %s", r.code, r.raw)
	}
	if r := call(t, admin, http.MethodPost, base0, `{"title": "PL49 eigene Nummer", "number": 9}`, nil); r.code != 422 {
		t.Fatalf("create with a number: %d %s", r.code, r.raw)
	}

	// The search finds it with and without the prefix.
	for _, q := range []string{"PRB-2", "2"} {
		r := call(t, admin, http.MethodGet, "/api/v1/search?types=probe.probe&q="+url.QueryEscape(q), "", nil)
		var hits []map[string]any
		if r.code != 200 || json.Unmarshal([]byte(r.raw), &hits) != nil {
			t.Fatalf("search %s: %d %s", q, r.code, r.raw)
		}
		found := false
		for _, h := range hits {
			found = found || h["id"] == id2
		}
		if !found {
			t.Fatalf("search %s does not find PRB-2: %s", q, r.raw)
		}
	}
	// A filter takes the number with its prefix.
	if r := call(t, admin, http.MethodPost, base0+":query", `{"where": {"field": "number", "eq": "PRB-2"}}`, nil); r.code != 200 ||
		len(r.body["items"].([]any)) != 1 || r.body["items"].([]any)[0].(map[string]any)["id"] != id2 {
		t.Fatalf("filter by PRB-2: %d %s", r.code, r.raw)
	}

	// An import may bring the old number; the sequence goes on above it, for the row of the same
	// batch without one too; the same row again keeps it, another number for it is refused.
	bulk := func(rows ...map[string]any) resp {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"source": "altsystem", "rows": rows})
		r := call(t, admin, http.MethodPost, base0+":bulk", string(raw), nil)
		if r.code != 200 {
			t.Fatalf("bulk: %d %s", r.code, r.raw)
		}
		return r
	}
	item := func(r resp, i int) map[string]any { return r.body["items"].([]any)[i].(map[string]any) }
	r := bulk(map[string]any{"external_key": "T-5000", "title": "Altfall", "number": "PRB-5000"},
		map[string]any{"external_key": "T-neu", "title": "Ohne Nummer"})
	imported := call(t, admin, http.MethodGet, base0+"/"+item(r, 0)["id"].(string), "", nil).body
	fresh := call(t, admin, http.MethodGet, base0+"/"+item(r, 1)["id"].(string), "", nil).body
	if imported["number"] != 5000.0 || fresh["number"] != 5001.0 {
		t.Fatalf("import: %v and %v", imported["number"], fresh["number"])
	}
	if next := create("PL49 nach dem Import"); next["number"] != 5002.0 {
		t.Fatalf("after the import: %v", next["number"])
	}
	r = bulk(map[string]any{"external_key": "T-5000", "title": "Altfall geändert", "number": 5000})
	if item(r, 0)["status"] != "updated" {
		t.Fatalf("same row again: %s", r.raw)
	}
	r = bulk(map[string]any{"external_key": "T-5000", "title": "Altfall", "number": 5002})
	if item(r, 0)["status"] != "error" {
		t.Fatalf("another number for the same row: %s", r.raw)
	}
}

// A type that gets a running number later (here through the customer layer): the records
// there are get theirs in the order of their creation, and the search finds them.
func TestSequenceForExistingRecords(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	t.Setenv("ITSM_TEST_DATABASE_URL", dbtest.Isolated(t, base, "itsmtest_api_pl49_later"))
	admin := apiServer(t)
	installPackage(t, t.Context(), admin.rt, customerPackage(t, "0.1.0", false))
	const things = "/api/v1/records/customertest.thing"
	var ids []string
	for _, title := range []string{"erstes", "zweites", "drittes"} {
		r := call(t, admin, http.MethodPost, things, `{"title": "`+title+`"}`, nil)
		if r.code != 201 {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
		ids = append(ids, r.body["id"].(string))
	}
	r := call(t, admin, http.MethodPost, "/api/v1/types/customertest.thing/customer",
		`{"version": 0, "change": {"op": "add_field", "field": "k_nummer", "new": {"name": "k_nummer", "type": "sequence", "label": {"de": "Nummer"}, "prefix": "D-"}}}`, nil)
	if r.code != 200 {
		t.Fatalf("add the field: %d %s", r.code, r.raw)
	}
	for i, id := range ids {
		if got := call(t, admin, http.MethodGet, things+"/"+id, "", nil).body["k_nummer"]; got != float64(i+1) {
			t.Fatalf("record %d: number %v", i+1, got)
		}
	}
	if next := call(t, admin, http.MethodPost, things, `{"title": "viertes"}`, nil); next.code != 201 || next.body["k_nummer"] != 4.0 {
		t.Fatalf("the next one: %d %s", next.code, next.raw)
	}
	s := call(t, admin, http.MethodGet, "/api/v1/search?types=customertest.thing&q=D-2", "", nil)
	var hits []map[string]any
	if s.code != 200 || json.Unmarshal([]byte(s.raw), &hits) != nil || len(hits) == 0 || hits[0]["id"] != ids[1] {
		t.Fatalf("search D-2: %d %s", s.code, s.raw)
	}
}
