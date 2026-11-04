// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package rules

import (
	"testing"
	"time"
)

// SD16d: today() is the date in Europe/Berlin, also shortly before midnight UTC; now() the
// moment in UTC; both with an offset, and in the conditions of rules and notifications.
func TestTodayAndNow(t *testing.T) {
	at := time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC) // 00:30 on the 5th in Berlin
	Clock = func() time.Time { return at }
	t.Cleanup(func() { Clock = time.Now })
	if got := Today(0); got != "2026-11-05" {
		t.Fatalf("today: %s", got)
	}
	if got := Today(-6); got != "2026-10-30" {
		t.Fatalf("today(-6): %s", got)
	}
	if got, err := Now("-24h"); err != nil || got != "2026-11-03T23:30:00Z" {
		t.Fatalf("now(-24h): %s %v", got, err)
	}
	if _, err := Now("gestern"); err == nil {
		t.Fatal("now() takes a duration")
	}
	for _, c := range []struct {
		expr string
		rec  map[string]any
		want bool
	}{
		{`record.expires_on < today()`, map[string]any{"expires_on": "2026-11-04"}, true},
		{`record.expires_on < today()`, map[string]any{"expires_on": "2026-11-05"}, false},
		{`record.expires_on == today(14)`, map[string]any{"expires_on": "2026-11-19"}, true},
		{`record.expires_on == null || record.expires_on >= today()`, map[string]any{"expires_on": nil}, true},
		{`record.updated_at < now("-1h")`, map[string]any{"updated_at": "2026-11-04T22:00:00Z"}, true},
		{`record.updated_at < now()`, map[string]any{"updated_at": "2026-11-05T08:00:00Z"}, false},
	} {
		cond, err := Compile(c.expr)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := cond.Eval(c.rec); err != nil || got != c.want {
			t.Errorf("%s on %v: %v %v, want %v", c.expr, c.rec, got, err, c.want)
		}
	}
	for _, compile := range []func(string) (*Condition, error){CompileRule, CompileNotify} {
		if _, err := compile(`record.expires_on == today(14)`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CompileClaims(`claims.since < today()`); err == nil {
		t.Fatal("the claims of a login know no today()")
	}
	cond, _ := Compile(`record.updated_at < now("gestern")`)
	if _, err := cond.Eval(map[string]any{"updated_at": "2026-11-04T22:00:00Z"}); err == nil {
		t.Fatal("a wrong duration is an error, not false")
	}
}
