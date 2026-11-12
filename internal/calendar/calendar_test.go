// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package calendar

import (
	"errors"
	"testing"
	"time"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// office is Monday to Friday 8–16 in Berlin.
func office(t *testing.T, holidays ...string) *Calendar {
	c := &Calendar{Location: berlin(t), Hours: map[time.Weekday][]Span{}, Holidays: map[string]bool{}}
	for d := time.Monday; d <= time.Friday; d++ {
		c.Hours[d] = []Span{{8 * 60, 16 * 60}}
	}
	for _, h := range holidays {
		c.Holidays[h] = true
	}
	return c
}

// The Prüfweg of WF8: 8 hours from Friday 15:00 end on Monday 15:00; a holiday on that
// Monday moves the end to Tuesday 15:00.
func TestAddOverTheWeekendAndAHoliday(t *testing.T) {
	loc := berlin(t)
	friday := time.Date(2026, 11, 6, 15, 0, 0, 0, loc)
	end, err := office(t).Add(friday, 8*time.Hour)
	if err != nil || !end.Equal(time.Date(2026, 11, 9, 15, 0, 0, 0, loc)) {
		t.Fatalf("Friday 15:00 + 8 h: %v %v", end, err)
	}
	end, err = office(t, "2026-11-09").Add(friday, 8*time.Hour)
	if err != nil || !end.Equal(time.Date(2026, 11, 10, 15, 0, 0, 0, loc)) {
		t.Fatalf("with a holiday on Monday: %v %v", end, err)
	}
}

func TestAddEdges(t *testing.T) {
	loc := berlin(t)
	c := office(t)
	cases := []struct {
		name  string
		start time.Time
		d     time.Duration
		want  time.Time
	}{
		{"before the day starts", time.Date(2026, 11, 9, 6, 0, 0, 0, loc), time.Hour, time.Date(2026, 11, 9, 9, 0, 0, 0, loc)},
		{"ends exactly at closing", time.Date(2026, 11, 9, 8, 0, 0, 0, loc), 8 * time.Hour, time.Date(2026, 11, 9, 16, 0, 0, 0, loc)},
		{"one minute more is the next day", time.Date(2026, 11, 9, 8, 0, 0, 0, loc), 8*time.Hour + time.Minute, time.Date(2026, 11, 10, 8, 1, 0, 0, loc)},
		{"on a Sunday", time.Date(2026, 11, 8, 12, 0, 0, 0, loc), 30 * time.Minute, time.Date(2026, 11, 9, 8, 30, 0, 0, loc)},
		{"zero", time.Date(2026, 11, 8, 12, 0, 0, 0, loc), 0, time.Date(2026, 11, 8, 12, 0, 0, 0, loc)},
		{"a start in UTC (15:30 in Berlin)", time.Date(2026, 11, 9, 14, 30, 0, 0, time.UTC), time.Hour, time.Date(2026, 11, 10, 8, 30, 0, 0, loc)},
	}
	for _, tc := range cases {
		got, err := c.Add(tc.start, tc.d)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%s: %v, want %v (%v)", tc.name, got, tc.want, err)
		}
	}
	// A lunch break and the change to winter time (25 October 2026 in Berlin).
	lunch := &Calendar{Location: loc, Hours: map[time.Weekday][]Span{time.Monday: {{13 * 60, 16 * 60}, {8 * 60, 12 * 60}}}}
	got, err := lunch.Add(time.Date(2026, 10, 19, 11, 0, 0, 0, loc), 2*time.Hour)
	if err != nil || !got.Equal(time.Date(2026, 10, 19, 14, 0, 0, 0, loc)) {
		t.Fatalf("over the lunch break: %v %v", got, err)
	}
	got, err = lunch.Add(time.Date(2026, 10, 23, 12, 0, 0, 0, loc), time.Hour)
	if err != nil || !got.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, loc)) || got.Hour() != 9 {
		t.Fatalf("over the change of the clocks: %v %v", got, err)
	}
	if _, err := (&Calendar{Location: loc}).Add(time.Now(), time.Hour); !errors.Is(err, ErrNoHours) {
		t.Fatalf("no hours: %v", err)
	}
	broken := &Calendar{Location: loc, Hours: map[time.Weekday][]Span{time.Monday: {{16 * 60, 8 * 60}}}}
	if _, err := broken.Add(time.Now(), time.Hour); err == nil {
		t.Fatal("hours that end before they start must be refused")
	}
}

func TestBetween(t *testing.T) {
	loc := berlin(t)
	c := office(t, "2026-11-09")
	got, err := c.Between(time.Date(2026, 11, 6, 15, 0, 0, 0, loc), time.Date(2026, 11, 10, 9, 30, 0, 0, loc))
	if err != nil || got != 2*time.Hour+30*time.Minute {
		t.Fatalf("Friday 15:00 to Tuesday 9:30 over a holiday: %v %v", got, err)
	}
	if got, _ := c.Between(time.Date(2026, 11, 10, 9, 0, 0, 0, loc), time.Date(2026, 11, 10, 8, 0, 0, 0, loc)); got != 0 {
		t.Fatalf("backwards: %v", got)
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int{"08:00": 480, "8:30": 510, "24:00": 1440, "00:00": 0} {
		if got, err := ParseClock(in); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, bad := range []string{"8", "8:5", "25:00", "24:01", "ab:cd", "08:60", ""} {
		if _, err := ParseClock(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestGermanHolidays(t *testing.T) {
	for year, want := range map[int]string{2024: "2024-03-31", 2025: "2025-04-20", 2026: "2026-04-05", 2027: "2027-03-28", 2038: "2038-04-25"} {
		if got := Easter(year).Format(time.DateOnly); got != want {
			t.Errorf("Easter %d: %s, want %s", year, got, want)
		}
	}
	count := func(state string, year int) map[string]string {
		t.Helper()
		list, err := GermanHolidays(year, state)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, h := range list {
			out[h.Date.Format(time.DateOnly)] = h.Name
		}
		return out
	}
	nw := count("NW", 2026)
	if len(nw) != 11 || nw["2026-06-04"] != "Fronleichnam" || nw["2026-11-01"] != "Allerheiligen" || nw["2026-04-03"] != "Karfreitag" {
		t.Fatalf("NW 2026: %v", nw)
	}
	if sn := count("SN", 2026); sn["2026-11-18"] != "Buß- und Bettag" || sn["2026-10-31"] != "Reformationstag" {
		t.Fatalf("SN 2026: %v", sn)
	}
	if be := count("BE", 2026); len(be) != 10 || be["2026-03-08"] == "" {
		t.Fatalf("BE 2026: %v", be)
	}
	if by := count("BY", 2017); by["2017-10-31"] == "" {
		t.Fatalf("2017 the Reformationstag was everywhere: %v", by)
	}
	if _, err := GermanHolidays(2026, "XX"); err == nil {
		t.Fatal("an unknown state must be refused")
	}
}

// WF7: a reminder two working days after Thursday 10:05 comes on Monday 10:05; a holiday on
// that Monday moves it to Tuesday; a Saturday counts from the Monday after it.
func TestAddDays(t *testing.T) {
	loc := berlin(t)
	thursday := time.Date(2026, 11, 12, 10, 5, 0, 0, loc)
	cases := []struct {
		name  string
		c     *Calendar
		start time.Time
		n     int
		want  time.Time
	}{
		{"two working days", office(t), thursday, 2, time.Date(2026, 11, 16, 10, 5, 0, 0, loc)},
		{"a holiday on Monday", office(t, "2026-11-16"), thursday, 2, time.Date(2026, 11, 17, 10, 5, 0, 0, loc)},
		{"from a Saturday", office(t), time.Date(2026, 11, 14, 9, 0, 0, 0, loc), 1, time.Date(2026, 11, 16, 9, 0, 0, 0, loc)},
		{"none", office(t), thursday, 0, thursday},
	}
	for _, c := range cases {
		got, err := c.c.AddDays(c.start, c.n)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("%s: %v %v, want %v", c.name, got, err, c.want)
		}
	}
	if _, err := (&Calendar{Location: loc, Hours: map[time.Weekday][]Span{}}).AddDays(thursday, 1); !errors.Is(err, ErrNoHours) {
		t.Errorf("a calendar without hours: %v", err)
	}
}
