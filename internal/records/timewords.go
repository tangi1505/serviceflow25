// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// --- time words in the JSON query (PL72, Kern-Plan E76) ---
//
// A condition on a time takes a word the query works out when it runs: an anchor @now (this
// moment), @today, @week, @month or @year (each its start, in the time zone of the person; a
// week starts on Monday) and after it, if wanted, one offset of sign, number and unit (m, h, d,
// w, mo, y). "Today" is then gte @today, "the last 5 minutes" gte @now-5m, "last month" gte
// @month-1mo and lt @month. A saved search keeps the word, so its window moves with the time.

// DefaultZone is the time zone of the anchors when the person's is not known (as in
// internal/prefs).
const DefaultZone = "Europe/Berlin"

var timeWord = regexp.MustCompile(`^@(now|today|week|month|year)(?:([+-])(\d{1,6})(mo|m|h|d|w|y))?$`)

// isTimeWordLike says whether a value is meant as a time word: a text that starts with @.
func isTimeWordLike(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && len(s) > 1 && s[0] == '@'
}

// resolveTimeWord works out a time word for the moment now in the zone loc. It says false for
// a text that is no time word.
func resolveTimeWord(word string, now time.Time, loc *time.Location) (time.Time, bool) {
	m := timeWord.FindStringSubmatch(word)
	if m == nil {
		return time.Time{}, false
	}
	t := now.In(loc)
	y, mon, d := t.Date()
	switch m[1] {
	case "today":
		t = time.Date(y, mon, d, 0, 0, 0, 0, loc)
	case "week":
		back := (int(t.Weekday()) + 6) % 7 // days since Monday
		t = time.Date(y, mon, d-back, 0, 0, 0, 0, loc)
	case "month":
		t = time.Date(y, mon, 1, 0, 0, 0, 0, loc)
	case "year":
		t = time.Date(y, time.January, 1, 0, 0, 0, 0, loc)
	}
	if m[2] == "" {
		return t, true
	}
	n, _ := strconv.Atoi(m[3]) // at most six digits, see timeWord
	if m[2] == "-" {
		n = -n
	}
	switch m[4] {
	case "m":
		t = t.Add(time.Duration(n) * time.Minute)
	case "h":
		t = t.Add(time.Duration(n) * time.Hour)
	case "d":
		t = t.AddDate(0, 0, n)
	case "w":
		t = t.AddDate(0, 0, 7*n)
	case "mo":
		t = addMonths(t, n)
	case "y":
		t = addMonths(t, 12*n)
	}
	return t, true
}

// addMonths moves t by n months and keeps the day within the month it lands in: 31 March
// minus a month is 28 or 29 February, not 3 March as with time.AddDate.
func addMonths(t time.Time, n int) time.Time {
	y, mon, d := t.Date()
	first := time.Date(y, mon+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := first.AddDate(0, 1, -1).Day()
	if d > last {
		d = last
	}
	return first.AddDate(0, 0, d-1)
}

// queryZone is the time zone the anchors of a query count in: the person's, through
// Service.Zone, else DefaultZone. The builder asks once and only when a word needs it.
func (s *Service) queryZone(ctx context.Context) *time.Location {
	if s.Zone != nil {
		if loc := s.Zone(ctx); loc != nil {
			return loc
		}
	}
	loc, err := time.LoadLocation(DefaultZone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// timeValue works out a time word for a condition on a time: a system time or a field of type
// datetime gets the moment, a field of type date the day in the person's zone. ok is false
// and a problem is noted when the text is no time word.
func (b *builder) timeValue(field, word string, date bool) (any, bool) {
	if b.zone == nil {
		b.zone = b.s.queryZone(b.ctx)
	}
	t, ok := resolveTimeWord(word, b.now(), b.zone)
	if !ok {
		b.problem(field, "time_word", fmt.Sprintf(
			"%q is no time word; words are @now, @today, @week, @month and @year, each with an offset like -5m, -1d, -1mo (m, h, d, w, mo, y)", word))
		return nil, false
	}
	if date {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), true
	}
	return t.UTC(), true
}

// now is the moment of the query; tests set Service.clock.
func (b *builder) now() time.Time {
	if b.s.clock != nil {
		return b.s.clock()
	}
	return time.Now()
}
