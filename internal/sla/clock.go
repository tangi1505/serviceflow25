// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package sla

import (
	"time"

	"github.com/google/uuid"
)

// The states of a clock.
const (
	StateRunning   = "running"
	StatePaused    = "paused"
	StateStopped   = "stopped"
	StateCancelled = "cancelled"
)

// Counter counts working time: a calendar (WF8), or round the clock.
type Counter interface {
	Add(start time.Time, d time.Duration) (time.Time, error)
	Between(a, b time.Time) (time.Duration, error)
}

// RoundTheClock counts every minute.
type RoundTheClock struct{}

// Add is start plus d.
func (RoundTheClock) Add(start time.Time, d time.Duration) (time.Time, error) {
	return start.Add(d), nil
}

// Between is b minus a, never below zero.
func (RoundTheClock) Between(a, b time.Time) (time.Duration, error) {
	if b.Before(a) {
		return 0, nil
	}
	return b.Sub(a), nil
}

// Clock is the deadline of one SLA at one record. Used is the working time counted up to
// ResumedAt, the start of the current run; while it runs, DueAt is when Used reaches Target.
type Clock struct {
	ID         uuid.UUID
	TypeName   string
	RecordID   uuid.UUID
	SLA        uuid.UUID
	State      string
	Target     time.Duration
	Calendar   uuid.UUID // uuid.Nil: round the clock
	StartedAt  time.Time
	Used       time.Duration
	ResumedAt  *time.Time
	DueAt      *time.Time
	BreachedAt *time.Time
	StoppedAt  *time.Time
	// Generation counts the changes; the job that waits for the next threshold carries it,
	// and a job of an older generation does nothing.
	Generation int64
	// Levels are the escalation levels (rule names) the clock reached, each once (WF10).
	Levels []string
}

// Level is an escalation level: a rule that fires once when a clock reaches Percent of its
// target (WF10), or Minutes of working time (WF11).
type Level struct {
	Rule    string
	Percent int
	Minutes int
}

// at is the working time at which a level fires.
func (l Level) at(target time.Duration) time.Duration {
	if l.Minutes > 0 {
		return time.Duration(l.Minutes) * time.Minute
	}
	return target * time.Duration(l.Percent) / 100
}

// reach adds the levels the clock reached with elapsed working time and returns them.
func (c *Clock) reach(levels []Level, elapsed time.Duration) []Level {
	var out []Level
	for _, l := range levels {
		if l.at(c.Target) <= elapsed && !c.reached(l.Rule) {
			c.Levels = append(c.Levels, l.Rule)
			out = append(out, l)
		}
	}
	return out
}

func (c *Clock) reached(rule string) bool {
	for _, r := range c.Levels {
		if r == rule {
			return true
		}
	}
	return false
}

// next is when a running clock next needs its job: at the lowest level not reached, or at the
// deadline if it is not past it yet; false when nothing is left to wait for.
func (c *Clock) next(levels []Level, n Counter) (time.Time, bool, error) {
	if c.State != StateRunning {
		return time.Time{}, false, nil
	}
	var wait time.Duration
	found := false
	consider := func(d time.Duration) {
		if !found || d < wait {
			wait, found = d, true
		}
	}
	if c.BreachedAt == nil {
		consider(c.Target)
	}
	for _, l := range levels {
		if !c.reached(l.Rule) {
			consider(l.at(c.Target))
		}
	}
	if !found {
		return time.Time{}, false, nil
	}
	at, err := n.Add(*c.ResumedAt, max(wait-c.Used, 0))
	return at, err == nil, err
}

// start makes the clock of an SLA that starts at StartedAt (the creation of the record, also
// when the SLA comes to hold later, WF-Plan E10) and is in state at now.
// Working time from start to now counts as used; it reports whether the clock is already past
// its target.
func start(c *Clock, state string, now time.Time, n Counter) (bool, error) {
	used, err := n.Between(c.StartedAt, now)
	if err != nil {
		return false, err
	}
	c.Used, c.State, c.ResumedAt, c.DueAt, c.BreachedAt, c.StoppedAt, c.Levels = used, StatePaused, nil, nil, nil, nil, nil
	c.Generation++
	breached := false
	if used >= c.Target {
		at, err := n.Add(c.StartedAt, c.Target)
		if err != nil {
			return false, err
		}
		c.BreachedAt, breached = &at, true
	}
	if state == StateRunning {
		if err := c.run(now, n); err != nil {
			return false, err
		}
	}
	c.State = state
	if state == StateStopped {
		c.StoppedAt = &now
	}
	return breached, nil
}

// move brings the clock to a state at now. Leaving a run adds its working time to Used; it
// reports whether the clock went past its target in that run (the job did not get there
// first).
func (c *Clock) move(to string, now time.Time, n Counter) (bool, error) {
	if c.State == to {
		return false, nil
	}
	breached := false
	if c.State == StateRunning {
		d, err := n.Between(*c.ResumedAt, now)
		if err != nil {
			return false, err
		}
		if c.BreachedAt == nil && c.Used+d >= c.Target {
			at := *c.DueAt
			c.BreachedAt, breached = &at, true
		}
		c.Used += d
		c.ResumedAt, c.DueAt = nil, nil
	}
	c.State = to
	c.Generation++
	switch to {
	case StateRunning:
		c.StoppedAt = nil
		if err := c.run(now, n); err != nil {
			return false, err
		}
	case StateStopped:
		c.StoppedAt = &now
	}
	return breached, nil
}

// run starts a run at now and works out when it is due.
func (c *Clock) run(now time.Time, n Counter) error {
	due, err := n.Add(now, max(c.Target-c.Used, 0))
	if err != nil {
		return err
	}
	c.ResumedAt, c.DueAt = &now, &due
	return nil
}

// Elapsed is the working time counted at now.
func (c *Clock) Elapsed(now time.Time, n Counter) (time.Duration, error) {
	if c.State != StateRunning {
		return c.Used, nil
	}
	d, err := n.Between(*c.ResumedAt, now)
	return c.Used + d, err
}
