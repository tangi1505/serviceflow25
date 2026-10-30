// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package sla

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/records"
)

// DueArgs is the job that waits for the deadline of one clock: River runs it at due_at.
type DueArgs struct {
	Clock      uuid.UUID `json:"clock"`
	Generation int64     `json:"generation"`
}

// Kind names the job.
func (DueArgs) Kind() string { return "sla.due" }

// Register adds the worker of the deadline jobs.
func (s *Service) Register(js *jobs.Service) {
	jobs.AddWorker(js, &dueWorker{s: s})
}

type dueWorker struct {
	river.WorkerDefaults[DueArgs]
	s *Service
}

// Work runs one deadline job.
func (w *dueWorker) Work(ctx context.Context, job *river.Job[DueArgs]) error {
	breached, err := w.s.Due(ctx, job.Args)
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"breached": breached})
}

// Due checks a clock at the time of its job. A job of an older generation, or for a clock
// that no longer runs, does nothing. Otherwise the clock notes the escalation levels it
// reached (WF10) and, past its target, the breach at the moment of its deadline; then it gets
// the job for what comes next. When nothing is reached yet (a holiday entered after the job
// was planned), it only gets a new job.
func (s *Service) Due(ctx context.Context, a DueArgs) (bool, error) {
	var c *Clock
	breached := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := queries.New(tx)
		row, err := q.ClockForUpdate(ctx, a.Clock)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("sla: clock: %w", err)
		}
		if row.Generation != a.Generation || row.State != StateRunning {
			return nil
		}
		c = fromRow(row)
		n, err := s.counter(ctx, c.Calendar)
		if err != nil {
			return err
		}
		due, err := n.Add(*c.ResumedAt, max(c.Target-c.Used, 0))
		if err != nil {
			return err
		}
		elapsed, err := c.Elapsed(s.Now(), n)
		if err != nil {
			return err
		}
		c.DueAt = &due
		c.Generation++
		if c.BreachedAt == nil && elapsed >= c.Target {
			c.BreachedAt, breached = &due, true
		}
		_, byID, err := s.load(ctx)
		if err != nil {
			return err
		}
		sla := byID[c.SLA]
		if sla == nil {
			// The SLA was switched off since; what the clock reached still counts.
			sla = &SLA{ID: c.SLA, Name: "?"}
		}
		return s.settle(ctx, tx, q, sla, c, true, n, elapsed, breached)
	})
	if err == nil && c != nil {
		s.rec.Touched(ctx, c.TypeName)
	}
	return breached, err
}

// --- reading ---

// View is a clock as the API shows it, at the time of the request.
type View struct {
	ID               uuid.UUID  `json:"id"`
	SLA              uuid.UUID  `json:"sla"`
	SLAName          string     `json:"sla_name"`
	State            string     `json:"state"`
	Breached         bool       `json:"breached"`
	Levels           []string   `json:"levels"`
	TargetMinutes    float64    `json:"target_minutes"`
	ElapsedMinutes   float64    `json:"elapsed_minutes"`
	RemainingMinutes float64    `json:"remaining_minutes"`
	Calendar         *uuid.UUID `json:"calendar"`
	StartedAt        time.Time  `json:"started_at"`
	DueAt            *time.Time `json:"due_at"`
	BreachedAt       *time.Time `json:"breached_at"`
	StoppedAt        *time.Time `json:"stopped_at"`
}

// Of returns the clocks of a record the caller may read, with the working time counted up to
// now; the remaining time is negative past the deadline.
func (s *Service) Of(ctx context.Context, typeName string, id uuid.UUID) ([]View, error) {
	if _, err := s.rec.Get(ctx, typeName, id); err != nil {
		return nil, err
	}
	rows, err := queries.New(s.pool).ClocksOfRecord(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("sla: clocks: %w", err)
	}
	_, byID, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	now := s.Now()
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		c := fromRow(r)
		n, err := s.counter(ctx, c.Calendar)
		if err != nil {
			return nil, err
		}
		elapsed, err := c.Elapsed(now, n)
		if err != nil {
			return nil, err
		}
		v := View{ID: c.ID, SLA: c.SLA, State: c.State, Breached: c.BreachedAt != nil, Levels: c.Levels,
			TargetMinutes: minutes(c.Target), ElapsedMinutes: minutes(elapsed), RemainingMinutes: minutes(c.Target - elapsed),
			StartedAt: c.StartedAt, DueAt: c.DueAt, BreachedAt: c.BreachedAt, StoppedAt: c.StoppedAt}
		if c.Calendar != uuid.Nil {
			v.Calendar = &c.Calendar
		}
		if sla := byID[c.SLA]; sla != nil {
			v.SLAName = sla.Name
		} else if r, err := s.rec.Get(system(ctx), TypeSLA, c.SLA); err == nil {
			v.SLAName = fmt.Sprint(r["name"])
		}
		out = append(out, v)
	}
	return out, nil
}

// minutes is a duration in minutes, to two places.
func minutes(d time.Duration) float64 {
	return math.Round(d.Minutes()*100) / 100
}

var _ records.Clocks = (*Service)(nil)
