// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package org

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Start from a tree without the nodes of this test (other runs leave them behind).
	if _, err := pool.Exec(ctx, "DELETE FROM org_node WHERE path <@ 'root.orgtest'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM org_setting WHERE path = 'root' AND key LIKE 'orgtest.%'"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestValidPath(t *testing.T) {
	for _, p := range []string{"root", "root.it", "root.it.support_2"} {
		if !ValidPath(p) {
			t.Errorf("%q should be valid", p)
		}
	}
	for _, p := range []string{"", "it", "root.", "root.IT", "root..it", "root.it-support"} {
		if ValidPath(p) {
			t.Errorf("%q should not be valid", p)
		}
	}
}

// The Prüfweg of PL6: a setting at the root, overridden at the child, is inherited by the
// grandchild from the child; removing the override makes the grandchild inherit from the root.
func TestSettingsInherit(t *testing.T) {
	pool := testPool(t)
	svc := New(pool)
	ctx := t.Context()

	if _, err := svc.Create(ctx, Root, "Orgtest", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad name: %v", err)
	}
	if _, err := svc.Create(ctx, "root.nope", "orgtest", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing parent: %v", err)
	}
	child, err := svc.Create(ctx, Root, "orgtest", map[string]string{"de": "Testabteilung"})
	if err != nil {
		t.Fatal(err)
	}
	if child.Path != "root.orgtest" || child.Parent != Root || child.Label["de"] != "Testabteilung" {
		t.Fatalf("child: %+v", child)
	}
	if _, err := svc.Create(ctx, Root, "orgtest", nil); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	grand, err := svc.Create(ctx, child.Path, "team", nil)
	if err != nil {
		t.Fatal(err)
	}

	set := func(path, key, value string) {
		t.Helper()
		if err := svc.SetSetting(ctx, path, key, json.RawMessage(value), nil); err != nil {
			t.Fatalf("set %s %s at %s: %v", key, value, path, err)
		}
	}
	set(Root, "orgtest.priority", `3`)
	set(Root, "orgtest.colour", `"blue"`)
	set(child.Path, "orgtest.priority", `1`)

	want := func(path, key, value, source string) {
		t.Helper()
		v, src, err := svc.Setting(ctx, path, key)
		if err != nil {
			t.Fatal(err)
		}
		if string(v) != value || src != source {
			t.Fatalf("%s at %s = %s from %s, want %s from %s", key, path, v, src, value, source)
		}
	}
	want(Root, "orgtest.priority", "3", Root)
	want(child.Path, "orgtest.priority", "1", child.Path)
	want(grand.Path, "orgtest.priority", "1", child.Path)
	want(grand.Path, "orgtest.colour", `"blue"`, Root)
	if v, _, err := svc.Setting(ctx, grand.Path, "orgtest.unknown"); err != nil || v != nil {
		t.Fatalf("unknown key: %s %v", v, err)
	}

	if err := svc.UnsetSetting(ctx, child.Path, "orgtest.priority"); err != nil {
		t.Fatal(err)
	}
	want(grand.Path, "orgtest.priority", "3", Root)
	if err := svc.UnsetSetting(ctx, child.Path, "orgtest.priority"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unset twice: %v", err)
	}
	if err := svc.SetSetting(ctx, child.Path, "Bad Key", json.RawMessage(`1`), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad key: %v", err)
	}
	if err := svc.SetSetting(ctx, child.Path, "orgtest.x", json.RawMessage(`{`), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad json: %v", err)
	}

	all, err := svc.Settings(ctx, grand.Path)
	if err != nil || len(all) < 2 {
		t.Fatalf("settings: %v %v", all, err)
	}

	if err := svc.Delete(ctx, child.Path); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("delete with children: %v", err)
	}
	if err := svc.Delete(ctx, Root); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete root: %v", err)
	}
	if err := svc.Delete(ctx, grand.Path); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, child.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Node(ctx, child.Path); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

// A subtree query on a type table uses the GiST index on org_path (architektur.md §5). The
// planner prefers a sequential scan on a tiny table, so the test asks it not to.
func TestSubtreeQueryUsesGistIndex(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'rec_probe_probe')").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Skip("probe package not installed in the test database")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // read-only transaction
	if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, "EXPLAIN SELECT id FROM rec_probe_probe WHERE org_path <@ 'root.orgtest'::ltree")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, line)
	}
	joined := strings.Join(plan, "\n")
	if !strings.Contains(joined, "rec_probe_probe_org_path_idx") {
		t.Fatalf("plan does not use the GiST index:\n%s", joined)
	}
}
