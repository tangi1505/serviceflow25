// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"testing"
)

// A list runs without JIT and without parallel workers (PL73), only inside its own
// transaction: the next query on the pool has the defaults again, and a list still pages.
func TestListWithoutJITAndParallelWorkers(t *testing.T) {
	s, ctx, pool := setup(t)
	defaults := func() (string, string) {
		t.Helper()
		var jit, workers string
		if err := pool.QueryRow(ctx, "SELECT current_setting('jit'), current_setting('max_parallel_workers_per_gather')").Scan(&jit, &workers); err != nil {
			t.Fatal(err)
		}
		return jit, workers
	}
	jit, workers := defaults()

	rows, done, err := listQuery(ctx, pool, "SELECT current_setting('jit'), current_setting('max_parallel_workers_per_gather')", nil)
	if err != nil {
		t.Fatal(err)
	}
	var inJIT, inWorkers string
	for rows.Next() {
		if err := rows.Scan(&inJIT, &inWorkers); err != nil {
			t.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	done()
	if inJIT != "off" || inWorkers != "0" {
		t.Fatalf("inside a list: jit %q, workers %q; want off and 0", inJIT, inWorkers)
	}
	// The pool hands out connections that left the list: the settings were local.
	for i := 0; i < 10; i++ {
		if j, w := defaults(); j != jit || w != workers {
			t.Fatalf("after a list: jit %q, workers %q; want %q and %q", j, w, jit, workers)
		}
	}
	// An error in the list ends its transaction; the next list works.
	if _, _, err := listQuery(ctx, pool, "SELECT * FROM no_such_table", nil); err == nil {
		t.Fatal("a broken list must fail")
	}
	for _, title := range []string{"list 1", "list 2", "list 3"} {
		if _, err := s.Create(ctx, testType, map[string]any{"title": title, "kind": "a"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	page, _, err := s.Query(ctx, testType, Query{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.Next == "" {
		t.Fatalf("first page: %v, %d items, next %q", err, len(page.Items), page.Next)
	}
	next, _, err := s.Query(ctx, testType, Query{Limit: 2, After: page.Next})
	if err != nil || len(next.Items) != 1 {
		t.Fatalf("second page: %v, %d items", err, len(next.Items))
	}
}
