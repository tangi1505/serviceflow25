// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package db

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"group24.de/itsm/internal/db/dbtest"
	"group24.de/itsm/internal/db/queries"
)

// testURL returns the database of the test run; tests skip without it. scripts/ci.sh sets it.
func testURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	return url
}

func TestMigrateIsRepeatableAndCreatesExtensions(t *testing.T) {
	url := testURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for range 2 {
		if err := Migrate(ctx, url, logger); err != nil {
			t.Fatal(err)
		}
	}

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, ext := range []string{"ltree", "pg_trgm"} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_extension WHERE extname = $1", ext).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("extension %s missing", ext)
		}
	}
	if _, err := queries.New(pool).DatabaseNow(ctx); err != nil {
		t.Fatalf("sqlc query: %v", err)
	}
}

// Several processes migrating River at once on an empty database, as two test packages did in
// the CI on 2026-11-02: every migration succeeds, River's tables are there once. The River step
// runs on its own here; within Migrate the goose lock before it staggers the processes, so that
// the race shows only now and then.
func TestMigrateRiverAtOnceOnAnEmptyDatabase(t *testing.T) {
	url := dbtest.Isolated(t, testURL(t), "itsmtest_db_at_once")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const n = 8
	errs := make(chan error, n)
	for range n {
		go func() { errs <- migrateRiver(ctx, url, logger) }()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var versions int
	if err := pool.QueryRow(ctx, "SELECT count(DISTINCT version) FROM river_migration").Scan(&versions); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM river_migration").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if versions == 0 || rows != versions {
		t.Errorf("river_migration has %d rows for %d versions", rows, versions)
	}
}

func TestOpenRejectsBadURL(t *testing.T) {
	if _, err := Open(context.Background(), "::not a url::"); err == nil {
		t.Fatal("expected an error")
	}
}
