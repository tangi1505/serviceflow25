// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package sla

import (
	"testing"
	"time"

	"group24.de/itsm/internal/calendar"
)

var t0 = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

func at(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }

func TestClockPauseAndResume(t *testing.T) {
	n := RoundTheClock{}
	c := &Clock{Target: time.Hour, StartedAt: at(0)}
	if breached, err := start(c, StateRunning, at(0), n); err != nil || breached || !c.DueAt.Equal(at(60)) || c.Generation != 1 {
		t.Fatalf("start: %v %v %+v", breached, err, c)
	}
	// The same state again is no change.
	if _, err := c.move(StateRunning, at(5), n); err != nil || c.Generation != 1 {
		t.Fatalf("no change: %+v", c)
	}
	if _, err := c.move(StatePaused, at(20), n); err != nil || c.Used != 20*time.Minute || c.DueAt != nil || c.ResumedAt != nil {
		t.Fatalf("pause: %+v", c)
	}
	if e, _ := c.Elapsed(at(45), n); e != 20*time.Minute {
		t.Fatalf("elapsed while paused: %v", e)
	}
	if _, err := c.move(StateRunning, at(50), n); err != nil || !c.DueAt.Equal(at(90)) {
		t.Fatalf("resume: %+v", c)
	}
	if e, _ := c.Elapsed(at(60), n); e != 30*time.Minute {
		t.Fatalf("elapsed after resuming: %v", e)
	}
	// Stopped after the deadline, before the job came: the stop notes the breach at the
	// deadline.
	breached, err := c.move(StateStopped, at(95), n)
	if err != nil || !breached || !c.BreachedAt.Equal(at(90)) || c.Used != 65*time.Minute || !c.StoppedAt.Equal(at(95)) {
		t.Fatalf("stop past the deadline: %v %v %+v", breached, err, c)
	}
	// Reopened, it goes on with what it counted, already breached.
	if breached, err := c.move(StateRunning, at(100), n); err != nil || breached || c.StoppedAt != nil || c.BreachedAt == nil {
		t.Fatalf("reopen: %v %v %+v", breached, err, c)
	}
}

func TestClockStartsLate(t *testing.T) {
	n := RoundTheClock{}
	// Counted from the creation 90 minutes ago, a target of 60 is past at once.
	c := &Clock{Target: time.Hour, StartedAt: at(0)}
	breached, err := start(c, StateRunning, at(90), n)
	if err != nil || !breached || !c.BreachedAt.Equal(at(60)) || c.Used != 90*time.Minute || !c.DueAt.Equal(at(90)) {
		t.Fatalf("late start: %v %v %+v", breached, err, c)
	}
	// Paused from the start, it does not run.
	c = &Clock{Target: time.Hour, StartedAt: at(0)}
	if _, err := start(c, StatePaused, at(10), n); err != nil || c.State != StatePaused || c.DueAt != nil || c.Used != 10*time.Minute {
		t.Fatalf("paused start: %v %+v", err, c)
	}
}

func TestClockWithCalendar(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	cal := &calendar.Calendar{Location: berlin, Hours: map[time.Weekday][]calendar.Span{}, Holidays: map[string]bool{"2026-11-09": true}}
	for d := time.Monday; d <= time.Friday; d++ {
		cal.Hours[d] = []calendar.Span{{From: 8 * 60, To: 16 * 60}}
	}
	// Friday 15:00 with 120 minutes, Monday a holiday: due on Tuesday at 9:00.
	fri := time.Date(2026, 11, 6, 15, 0, 0, 0, berlin)
	c := &Clock{Target: 2 * time.Hour, StartedAt: fri}
	if _, err := start(c, StateRunning, fri, cal); err != nil || !c.DueAt.Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, berlin)) {
		t.Fatalf("with a calendar: %v %+v", err, c)
	}
	// Paused over the weekend, resumed on Tuesday at 8:00: the hour of Friday counts.
	if _, err := c.move(StatePaused, time.Date(2026, 11, 7, 10, 0, 0, 0, berlin), cal); err != nil || c.Used != time.Hour {
		t.Fatalf("paused on Saturday: %v %+v", err, c)
	}
	if _, err := c.move(StateRunning, time.Date(2026, 11, 10, 8, 0, 0, 0, berlin), cal); err != nil ||
		!c.DueAt.Equal(time.Date(2026, 11, 10, 9, 0, 0, 0, berlin)) {
		t.Fatalf("resumed on Tuesday: %v %+v", err, c)
	}
}

func TestClockLevels(t *testing.T) {
	n := RoundTheClock{}
	levels := []Level{{Rule: "s1", Percent: 80}, {Rule: "s2", Percent: 100}, {Rule: "s3", Percent: 150}}
	c := &Clock{Target: time.Hour, StartedAt: at(0)}
	if _, err := start(c, StateRunning, at(0), n); err != nil {
		t.Fatal(err)
	}
	if next, ok, _ := c.next(levels, n); !ok || !next.Equal(at(48)) {
		t.Fatalf("first threshold: %v %v", next, ok)
	}
	// Reached levels fire once; the deadline comes before the next level.
	if got := c.reach(levels, 50*time.Minute); len(got) != 1 || got[0].Rule != "s1" || len(c.reach(levels, 50*time.Minute)) != 0 {
		t.Fatalf("reach at 50: %v %v", got, c.Levels)
	}
	if next, _, _ := c.next(levels, n); !next.Equal(at(60)) {
		t.Fatalf("after level 1: %v", next)
	}
	// Breached and past level 2, only level 3 is left; after a pause the run starts later.
	c.reach(levels, time.Hour)
	breached := at(60)
	c.BreachedAt = &breached
	if _, err := c.move(StatePaused, at(70), n); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.next(levels, n); ok {
		t.Fatal("a paused clock has no job")
	}
	if _, err := c.move(StateRunning, at(100), n); err != nil {
		t.Fatal(err)
	}
	if next, ok, _ := c.next(levels, n); !ok || !next.Equal(at(120)) {
		t.Fatalf("level 3 after the pause: %v %v", next, ok)
	}
	// All levels reached and breached: nothing left to wait for.
	c.reach(levels, 2*time.Hour)
	if _, ok, _ := c.next(levels, n); ok {
		t.Fatal("nothing left, still a job")
	}
	// A clock started anew (a cancelled one whose SLA holds again) reaches its levels anew.
	if _, err := start(c, StateRunning, at(200), n); err != nil || len(c.Levels) != 0 {
		t.Fatalf("restart keeps levels: %v", c.Levels)
	}
}

func TestClockLevelsInMinutes(t *testing.T) {
	n := RoundTheClock{}
	// Seven days to wait, reminders after two and five days (WF11): minutes count, not shares.
	week := 7 * 24 * time.Hour
	levels := []Level{{Rule: "r1", Minutes: 2 * 24 * 60}, {Rule: "r2", Minutes: 5 * 24 * 60}, {Rule: "end", Percent: 100}}
	c := &Clock{Target: week, StartedAt: at(0)}
	if _, err := start(c, StateRunning, at(0), n); err != nil {
		t.Fatal(err)
	}
	if next, ok, _ := c.next(levels, n); !ok || !next.Equal(at(2*24*60)) {
		t.Fatalf("first reminder: %v %v", next, ok)
	}
	if got := c.reach(levels, 5*24*time.Hour); len(got) != 2 {
		t.Fatalf("both reminders at five days: %v", got)
	}
	if next, _, _ := c.next(levels, n); !next.Equal(at(7 * 24 * 60)) {
		t.Fatalf("the end after seven days: %v", next)
	}
}
