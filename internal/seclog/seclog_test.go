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
	"group24.de/itsm/internal/db/dbtest"
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
	l := New(db.Single(pool), func(context.Context, string, string) (json.RawMessage, string, error) { return days, "root", nil }, quiet)
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

// The ids of the security log stay in order when the clock of the database steps back (PL47e,
// migration 40): a later entry gets at least the last id plus one. The step back is played by
// setting the last id far ahead; a database of its own keeps that from the other tests.
func TestIDsStayInOrderWhenTheClockStepsBack(t *testing.T) {
	base := os.Getenv("ITSM_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	url := dbtest.Isolated(t, base, "itsmtest_seclog_order")
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	l := New(db.Single(pool), nil, quiet)
	l.Record(ctx, Event{Category: Change, Kind: "test.first", Outcome: Success})
	// An hour ahead of the clock: as if the clock had stepped back by an hour since.
	ahead := (time.Now().Add(time.Hour).UnixMicro() << 10) | 1023
	if _, err := pool.Exec(ctx, "UPDATE security_event_last SET id = $1", ahead); err != nil {
		t.Fatal(err)
	}
	l.Record(ctx, Event{Category: Change, Kind: "test.second", Outcome: Success})
	l.Record(ctx, Event{Category: Change, Kind: "test.third", Outcome: Success})
	page, err := l.List(ctx, Filter{Category: Change, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.Items[0].Kind != "test.third" || page.Items[1].Kind != "test.second" || page.Items[2].Kind != "test.first" {
		t.Fatalf("newest first: %+v", page.Items)
	}
	if page.Items[1].ID != ahead+1 || page.Items[0].ID != ahead+2 {
		t.Fatalf("ids after the step back: %d, %d; want %d, %d", page.Items[1].ID, page.Items[0].ID, ahead+1, ahead+2)
	}
}
