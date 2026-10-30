// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package seclog

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"group24.de/itsm/internal/db"
)

// Retention: the setting decides, the default is a year; Expire removes what is older and
// keeps the rest.
func TestRetention(t *testing.T) {
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	days := json.RawMessage("30")
	l := New(pool, func(context.Context, string, string) (json.RawMessage, string, error) { return days, "root", nil }, quiet)
	if got := l.Retention(ctx); got != 30*24*time.Hour {
		t.Fatalf("retention from the setting: %v", got)
	}
	days = json.RawMessage(`"x"`)
	if got := l.Retention(ctx); got != DefaultRetention {
		t.Fatalf("a bad setting falls back: %v", got)
	}
	days = json.RawMessage("30")

	target := "seclogtest-" + time.Now().Format("150405.000000")
	for _, kind := range []string{"old", "fresh"} {
		l.Record(ctx, Event{Category: Change, Kind: "test." + kind, Outcome: Success, Target: target})
	}
	if _, err := pool.Exec(ctx, "UPDATE security_event SET at = now() - interval '31 days' WHERE target = $1 AND kind = 'test.old'", target); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	rows, err := pool.Query(ctx, "SELECT kind FROM security_event WHERE target = $1", target)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		kinds = append(kinds, k)
	}
	rows.Close()
	if len(kinds) != 1 || kinds[0] != "test.fresh" {
		t.Fatalf("after expire: %v", kinds)
	}
	var nilLog *Log
	nilLog.Record(ctx, Event{}) // a nil log records nothing and does not panic
}
