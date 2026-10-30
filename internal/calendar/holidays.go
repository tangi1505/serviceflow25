// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package calendar

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// Holiday is a day off with its name.
type Holiday struct {
	Date time.Time
	Name string
}

// States are the German states by their usual code.
var States = []string{"BW", "BY", "BE", "BB", "HB", "HH", "HE", "MV", "NI", "NW", "RP", "SL", "SN", "ST", "SH", "TH"}

// Easter returns Easter Sunday of a year in the Gregorian calendar (anonymous algorithm of
// Meeus, Jones and Butcher).
func Easter(year int) time.Time {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// GermanHolidays lists the statutory public holidays of a German state in a year, by date.
// Days that only some municipalities keep (Mariä Himmelfahrt in Bavaria, Fronleichnam in parts
// of Saxony and Thuringia, the Augsburger Friedensfest) are left out; they can be added by
// hand.
func GermanHolidays(year int, state string) ([]Holiday, error) {
	if !slices.Contains(States, state) {
		return nil, fmt.Errorf("calendar: %q is no German state; use one of %v", state, States)
	}
	if year < 1995 || year > 2200 {
		return nil, fmt.Errorf("calendar: year %d is out of range", year)
	}
	in := func(states ...string) bool { return slices.Contains(states, state) }
	date := func(m time.Month, d int) time.Time { return time.Date(year, m, d, 0, 0, 0, 0, time.UTC) }
	easter := Easter(year)
	out := []Holiday{
		{date(time.January, 1), "Neujahr"},
		{easter.AddDate(0, 0, -2), "Karfreitag"},
		{easter.AddDate(0, 0, 1), "Ostermontag"},
		{date(time.May, 1), "Tag der Arbeit"},
		{easter.AddDate(0, 0, 39), "Christi Himmelfahrt"},
		{easter.AddDate(0, 0, 50), "Pfingstmontag"},
		{date(time.October, 3), "Tag der Deutschen Einheit"},
		{date(time.December, 25), "1. Weihnachtstag"},
		{date(time.December, 26), "2. Weihnachtstag"},
	}
	add := func(ok bool, d time.Time, name string) {
		if ok {
			out = append(out, Holiday{d, name})
		}
	}
	add(in("BW", "BY", "ST"), date(time.January, 6), "Heilige Drei Könige")
	add(in("BE") && year >= 2019 || in("MV") && year >= 2023, date(time.March, 8), "Internationaler Frauentag")
	add(in("BB"), easter, "Ostersonntag")
	add(in("BB"), easter.AddDate(0, 0, 49), "Pfingstsonntag")
	add(in("BW", "BY", "HE", "NW", "RP", "SL"), easter.AddDate(0, 0, 60), "Fronleichnam")
	add(in("SL"), date(time.August, 15), "Mariä Himmelfahrt")
	add(in("TH") && year >= 2019, date(time.September, 20), "Weltkindertag")
	add(in("BB", "MV", "SN", "ST", "TH") || in("HB", "HH", "NI", "SH") && year >= 2018 || year == 2017,
		date(time.October, 31), "Reformationstag")
	add(in("BW", "BY", "NW", "RP", "SL"), date(time.November, 1), "Allerheiligen")
	if in("SN") {
		// Buß- und Bettag: the Wednesday before 23 November.
		d := date(time.November, 22)
		for d.Weekday() != time.Wednesday {
			d = d.AddDate(0, 0, -1)
		}
		out = append(out, Holiday{d, "Buß- und Bettag"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}
