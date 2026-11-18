// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// The time words of PL72 with a fixed clock: anchors in the person's zone, the week from
// Monday, months that keep their last day, the change to summer time, and words that are none.
func TestResolveTimeWord(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, berlin)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		word, now, want string
	}{
		// 00:30 in Berlin is still the day before in UTC; today starts at 00:00 in Berlin.
		{"@today", "2026-11-18 00:30", "2026-11-18 00:00"},
		{"@today-1d", "2026-11-18 00:30", "2026-11-17 00:00"},
		{"@now-5m", "2026-11-18 10:00", "2026-11-18 09:55"},
		{"@now+2h", "2026-11-18 10:00", "2026-11-18 12:00"},
		{"@month-1mo", "2026-03-31 12:00", "2026-02-01 00:00"},
		{"@month", "2026-03-31 12:00", "2026-03-01 00:00"},
		// A month back from 31 March lands on the last day of February, not on 3 March.
		{"@now-1mo", "2026-03-31 12:00", "2026-02-28 12:00"},
		{"@now-1y", "2028-02-29 08:00", "2027-02-28 08:00"},
		// 2026-11-22 is a Sunday: the week started on Monday the 16th.
		{"@week", "2026-11-22 23:00", "2026-11-16 00:00"},
		{"@week", "2026-11-16 00:00", "2026-11-16 00:00"},
		{"@week-1w", "2026-11-18 09:00", "2026-11-09 00:00"},
		{"@year", "2026-11-18 09:00", "2026-01-01 00:00"},
		{"@year+1y", "2026-11-18 09:00", "2027-01-01 00:00"},
		// Across the change to summer time (29 March 2026) a day is a day of the calendar.
		{"@today-1d", "2026-03-30 08:00", "2026-03-29 00:00"},
		{"@now-1d", "2026-03-30 08:00", "2026-03-29 08:00"},
	}
	for _, c := range cases {
		got, ok := resolveTimeWord(c.word, at(c.now), berlin)
		if !ok || !got.Equal(at(c.want)) {
			t.Errorf("%s at %s = %v, %v; want %s", c.word, c.now, got.In(berlin), ok, c.want)
		}
	}
	for _, bad := range []string{"@heute", "@now-5", "@now-5x", "@today -1d", "@now--1d", "@now-1d-1h", "now", "@", "@NOW"} {
		if _, ok := resolveTimeWord(bad, at("2026-11-18 10:00"), berlin); ok {
			t.Errorf("%q is taken as a time word", bad)
		}
	}
}

// The words in a real query: on a system time, a datetime field and a date field, in the
// zone of the person (Service.Zone); an unknown word is a problem of the input with its field.
func TestQueryTimeWords(t *testing.T) {
	s, ctx, _ := setup(t)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// 00:30 on 18 November in Berlin, 23:30 on the 17th in UTC.
	now := time.Date(2026, 11, 18, 0, 30, 0, 0, berlin)
	s.clock = func() time.Time { return now }
	s.Zone = func(context.Context) *time.Location { return berlin }
	t.Cleanup(func() { s.clock, s.Zone = nil, nil })

	mk := func(title, due, seen string) {
		t.Helper()
		if _, err := s.Create(ctx, testType, map[string]any{"title": title, "kind": "a", "due_on": due, "seen_at": seen}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Seen at 23:50 on the 17th in Berlin (yesterday), and at 00:10 on the 18th (today).
	mk("yesterday", "2026-11-17", "2026-11-17T22:50:00Z")
	mk("today", "2026-11-18", "2026-11-17T23:10:00Z")
	mk("last month", "2026-10-05", "2026-10-05T10:00:00Z")

	titles := func(where string) []string {
		t.Helper()
		var q Query
		if err := json.Unmarshal([]byte(`{"where": `+where+`, "sort": ["title"]}`), &q); err != nil {
			t.Fatal(err)
		}
		page, _, err := s.Query(ctx, testType, q)
		if err != nil {
			t.Fatalf("%s: %v", where, err)
		}
		var out []string
		for _, it := range page.Items {
			out = append(out, it["title"].(string))
		}
		return out
	}
	check := func(where string, want ...string) {
		t.Helper()
		got := titles(where)
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", where, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s = %v, want %v", where, got, want)
			}
		}
	}
	// "Today" in Berlin does not take 23:50 of yesterday, though it is the same day in UTC.
	check(`{"field": "seen_at", "gte": "@today"}`, "today")
	check(`{"and": [{"field": "seen_at", "gte": "@today-1d"}, {"field": "seen_at", "lt": "@today"}]}`, "yesterday")
	check(`{"and": [{"field": "seen_at", "gte": "@month-1mo"}, {"field": "seen_at", "lt": "@month"}]}`, "last month")
	// A date field compares with the day in the person's zone.
	check(`{"field": "due_on", "eq": "@today"}`, "today")
	check(`{"field": "due_on", "lt": "@week"}`, "last month")
	// A system time, on the real clock: the three records were created a moment ago.
	s.clock = nil
	check(`{"field": "created_at", "gte": "@now-5m"}`, "last month", "today", "yesterday")
	check(`{"field": "created_at", "gte": "@now+1h"}`)
	s.clock = func() time.Time { return now }

	var q Query
	if err := json.Unmarshal([]byte(`{"where": {"field": "seen_at", "gte": "@heute"}}`), &q); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.Query(ctx, testType, q)
	var bad *ValidationError
	if !errors.As(err, &bad) || len(bad.Problems) != 1 || bad.Problems[0].Field != "seen_at" || bad.Problems[0].Code != "time_word" {
		t.Fatalf("@heute = %v, want a problem on seen_at", err)
	}
	// A text field keeps a value that starts with @ as text.
	check(`{"field": "title", "eq": "@today"}`)
}
