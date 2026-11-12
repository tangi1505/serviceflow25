// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package calendar counts working time (WF8; Inventar G169; Hauptplan E113): a calendar has
// hours per weekday, possibly several spans a day, and holidays without working time, in its
// own time zone. Add moves a moment on by a working duration (a deadline of 8 hours from
// Friday 15:00 with hours 8–16 ends on Monday at 15:00), Between counts the working time
// between two moments (the remaining time of an SLA clock, WF9). Calendars, hours and holidays
// are records of the core package; Service loads them and generates German public holidays.
package calendar

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNoHours says a calendar has no working time at all: no deadline could ever end.
var ErrNoHours = errors.New("calendar: no working hours")

// maxDays bounds a search for working time, ten years of days.
const maxDays = 3660

// Span is working time on a day, in minutes after midnight, from inclusive to exclusive.
type Span struct{ From, To int }

// Calendar is what the counting needs.
type Calendar struct {
	Location *time.Location
	Hours    map[time.Weekday][]Span
	// Holidays are dates "2006-01-02" without working time.
	Holidays map[string]bool
}

// ParseClock reads "HH:MM" as minutes after midnight; "24:00" is the end of the day.
func ParseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || len(m) != 2 || hh < 0 || mm < 0 || mm > 59 || hh > 24 || (hh == 24 && mm != 0) {
		return 0, fmt.Errorf("calendar: %q is no time of day like 08:00", s)
	}
	return hh*60 + mm, nil
}

// normalise sorts the spans of each day and merges overlapping ones.
func (c *Calendar) normalise() error {
	total := 0
	for d, spans := range c.Hours {
		sort.Slice(spans, func(i, j int) bool { return spans[i].From < spans[j].From })
		var out []Span
		for _, s := range spans {
			if s.To <= s.From {
				return fmt.Errorf("calendar: working hours must end after they start (%02d:%02d–%02d:%02d)", s.From/60, s.From%60, s.To/60, s.To%60)
			}
			if n := len(out); n > 0 && s.From <= out[n-1].To {
				out[n-1].To = max(out[n-1].To, s.To)
				continue
			}
			out = append(out, s)
		}
		c.Hours[d] = out
		total += len(out)
	}
	if c.Location == nil {
		c.Location = time.UTC
	}
	if total == 0 {
		return ErrNoHours
	}
	return nil
}

// day is one date's working spans as moments; none on a holiday.
func (c *Calendar) day(date time.Time) [][2]time.Time {
	y, m, d := date.Date()
	if c.Holidays[fmt.Sprintf("%04d-%02d-%02d", y, m, d)] {
		return nil
	}
	spans := c.Hours[date.Weekday()]
	out := make([][2]time.Time, 0, len(spans))
	for _, s := range spans {
		from := time.Date(y, m, d, s.From/60, s.From%60, 0, 0, c.Location)
		to := time.Date(y, m, d, s.To/60, s.To%60, 0, 0, c.Location)
		out = append(out, [2]time.Time{from, to})
	}
	return out
}

// Add returns the moment that lies the working duration d after start. Outside working time
// the count starts at the next working moment; d of zero or less returns start.
func (c *Calendar) Add(start time.Time, d time.Duration) (time.Time, error) {
	if err := c.normalise(); err != nil {
		return time.Time{}, err
	}
	if d <= 0 {
		return start, nil
	}
	cur := start.In(c.Location)
	date := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, c.Location)
	for range maxDays {
		for _, span := range c.day(date) {
			begin := span[0]
			if cur.After(begin) {
				begin = cur
			}
			if !begin.Before(span[1]) {
				continue
			}
			avail := span[1].Sub(begin)
			if d <= avail {
				return begin.Add(d), nil
			}
			d -= avail
		}
		date = date.AddDate(0, 0, 1)
	}
	return time.Time{}, fmt.Errorf("%w in the next ten years", ErrNoHours)
}

// Between returns the working time from a to b (zero when b is not after a).
func (c *Calendar) Between(a, b time.Time) (time.Duration, error) {
	if err := c.normalise(); err != nil {
		return 0, err
	}
	if !b.After(a) {
		return 0, nil
	}
	a, b = a.In(c.Location), b.In(c.Location)
	date := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, c.Location)
	var sum time.Duration
	for range maxDays {
		if date.After(b) {
			break
		}
		for _, span := range c.day(date) {
			from, to := span[0], span[1]
			if a.After(from) {
				from = a
			}
			if b.Before(to) {
				to = b
			}
			if to.After(from) {
				sum += to.Sub(from)
			}
		}
		date = date.AddDate(0, 0, 1)
	}
	return sum, nil
}

// AddDays returns the moment at the time of day of start on the n-th working day after the day
// of start: a day with working hours that is no holiday (WF7: a reminder two working days
// later, Thursday 10:05 to Monday 10:05). n of zero or less returns start.
func (c *Calendar) AddDays(start time.Time, n int) (time.Time, error) {
	if err := c.normalise(); err != nil {
		return time.Time{}, err
	}
	if n <= 0 {
		return start, nil
	}
	cur := start.In(c.Location)
	date := time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, c.Location)
	for range maxDays {
		date = date.AddDate(0, 0, 1)
		if len(c.day(date)) == 0 {
			continue
		}
		if n--; n == 0 {
			return time.Date(date.Year(), date.Month(), date.Day(), cur.Hour(), cur.Minute(), cur.Second(), 0, c.Location), nil
		}
	}
	return time.Time{}, fmt.Errorf("%w in the next ten years", ErrNoHours)
}
