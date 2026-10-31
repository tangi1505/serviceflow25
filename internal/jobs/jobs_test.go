// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type sleepArgs struct {
	Millis int `json:"millis"`
}

func (sleepArgs) Kind() string { return "jobstest.sleep" }

type sleepWorker struct {
	river.WorkerDefaults[sleepArgs]
}

func (sleepWorker) Work(ctx context.Context, job *river.Job[sleepArgs]) error {
	select {
	case <-time.After(time.Duration(job.Args.Millis) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type tickArgs struct{}

func (tickArgs) Kind() string { return "jobstest.tick" }

type tickWorker struct {
	river.WorkerDefaults[tickArgs]
}

func (tickWorker) Work(context.Context, *river.Job[tickArgs]) error { return nil }

func setup(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		"DELETE FROM river_job WHERE kind LIKE 'jobstest.%'",
		"DELETE FROM job_schedule WHERE name LIKE 'jobstest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, pool
}

// waitFor returns when the job with the id completes, or fails the test after the limit.
// completions collects the completion times of jobs; waiting for one job keeps the events
// of the others, so that a job finishing early (under load) is not lost.
type completions struct {
	events <-chan *river.Event
	seen   map[int64]time.Time
}

func watch(events <-chan *river.Event) *completions {
	return &completions{events: events, seen: map[int64]time.Time{}}
}

func waitFor(t *testing.T, c *completions, id int64, limit time.Duration) time.Time {
	t.Helper()
	if at, ok := c.seen[id]; ok {
		return at
	}
	deadline := time.After(limit)
	for {
		select {
		case e := <-c.events:
			c.seen[e.Job.ID] = time.Now()
			if e.Job.ID == id {
				return c.seen[id]
			}
		case <-deadline:
			t.Fatalf("job %d did not complete within %s", id, limit)
		}
	}
}

// Test queues no running server works (the dev stack shares the database).
const (
	queueFast = "jobstest_interactive"
	queueSlow = "jobstest_bulk"
)

// The acceptance of PL23: six jobs saturate the two workers of the slow queue for about
// three seconds; a job in the fast queue, inserted after them, still completes at once. The
// queues stand for bulk and interactive; only their names differ, so that no server fetches
// the test jobs.
func TestBulkDoesNotDelayInteractive(t *testing.T) {
	ctx, pool := setup(t)
	s := New(db.Single(pool), Options{Logger: quiet, Workers: map[string]int{queueFast: 4, queueSlow: 2}})
	AddWorker(s, sleepWorker{})
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	events, cancel := s.clients[""].Subscribe(river.EventKindJobCompleted)
	defer cancel()
	done := watch(events)

	start := time.Now()
	var bulk []int64
	for range 6 {
		j, err := s.Enqueue(ctx, sleepArgs{Millis: 1000}, &river.InsertOpts{Queue: queueSlow})
		if err != nil {
			t.Fatal(err)
		}
		bulk = append(bulk, j.ID)
	}
	fast, err := s.Enqueue(ctx, sleepArgs{Millis: 10}, &river.InsertOpts{Queue: queueFast})
	if err != nil {
		t.Fatal(err)
	}
	fastDone := waitFor(t, done, fast.ID, 5*time.Second)
	if d := fastDone.Sub(start); d > 1500*time.Millisecond {
		t.Fatalf("interactive job waited behind bulk: %s", d)
	}
	// The bulk queue really was saturated: with two workers six seconds of sleep take at
	// least three seconds of wall time.
	var lastBulk time.Time
	for _, id := range bulk {
		if at := waitFor(t, done, id, 15*time.Second); at.After(lastBulk) {
			lastBulk = at
		}
	}
	if d := lastBulk.Sub(start); d < 2900*time.Millisecond {
		t.Fatalf("bulk queue was not limited to two workers: all done after %s", d)
	}
	list, err := s.List(ctx, Filter{Queue: queueSlow, Kind: "jobstest.sleep", State: "completed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var args sleepArgs
	if len(list) == 6 {
		_ = json.Unmarshal(list[0].Args, &args)
	}
	if len(list) != 6 || list[0].Queue != queueSlow || list[0].FinalizedAt == nil || list[0].State != "completed" || args.Millis != 1000 {
		t.Fatalf("log of the bulk jobs: %d rows, first %s %s args %s", len(list), list[0].Queue, list[0].State, list[0].Args)
	}
	if _, err := s.List(ctx, Filter{State: "sleeping"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown state must be invalid: %v", err)
	}
}

// Schedules: the default of the code, the override of the table, next and last run, run now.
func TestSchedulesOverrideAndRunNow(t *testing.T) {
	ctx, pool := setup(t)
	s := New(db.Single(pool), Options{Logger: quiet, Workers: map[string]int{queueFast: 1}})
	AddWorker(s, tickWorker{})
	tick := Periodic{Name: "jobstest.tick", Label: map[string]string{"de": "Tick"}, Schedule: "*/5 * * * *", Queue: queueFast,
		Args: func() river.JobArgs { return tickArgs{} }}
	if err := s.AddPeriodic(tick); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPeriodic(tick); !errors.Is(err, ErrInvalid) {
		t.Fatalf("twice must fail: %v", err)
	}
	bad := tick
	bad.Name, bad.Schedule = "jobstest.bad", "every day"
	if err := s.AddPeriodic(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad schedule must fail: %v", err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })

	sched, err := s.ScheduleByName(ctx, "jobstest.tick")
	if err != nil {
		t.Fatal(err)
	}
	if !sched.Enabled || sched.Schedule != "*/5 * * * *" || sched.DefaultSchedule != "*/5 * * * *" || sched.NextRun == nil ||
		time.Until(*sched.NextRun) > 5*time.Minute || sched.LastRun != nil || sched.Label["de"] != "Tick" {
		t.Fatalf("default schedule: %+v", sched)
	}

	actor := uuid.New()
	off := false
	sched, err = s.SetSchedule(ctx, "jobstest.tick", SchedulePatch{Enabled: &off}, &actor)
	if err != nil {
		t.Fatal(err)
	}
	if sched.Enabled || sched.NextRun != nil || sched.UpdatedBy == nil || *sched.UpdatedBy != actor || sched.Schedule != "*/5 * * * *" {
		t.Fatalf("switched off: %+v", sched)
	}
	nightly := "0 3 * * *"
	sched, err = s.SetSchedule(ctx, "jobstest.tick", SchedulePatch{Schedule: &nightly}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sched.Enabled || sched.Schedule != nightly || sched.DefaultSchedule != "*/5 * * * *" {
		t.Fatalf("override keeps enabled=false: %+v", sched)
	}
	on := true
	sched, err = s.SetSchedule(ctx, "jobstest.tick", SchedulePatch{Enabled: &on}, nil)
	if err != nil || !sched.Enabled || sched.NextRun == nil || sched.NextRun.Hour() != 3 || sched.NextRun.Minute() != 0 {
		t.Fatalf("nightly: %+v %v", sched, err)
	}
	wrong := "61 * * * *"
	if _, err := s.SetSchedule(ctx, "jobstest.tick", SchedulePatch{Schedule: &wrong}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad schedule: %v", err)
	}
	if _, err := s.SetSchedule(ctx, "jobstest.none", SchedulePatch{Enabled: &on}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown schedule: %v", err)
	}
	row, err := queries.New(pool).GetJobSchedule(ctx, "jobstest.tick")
	if err != nil || row.Schedule != nightly || !row.Enabled {
		t.Fatalf("table: %+v %v", row, err)
	}

	events, cancel := s.clients[""].Subscribe(river.EventKindJobCompleted)
	defer cancel()
	done := watch(events)
	job, err := s.RunNow(ctx, "jobstest.tick")
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != "jobstest.tick" || job.Queue != queueFast {
		t.Fatalf("run now: %+v", job)
	}
	waitFor(t, done, job.ID, 5*time.Second)
	sched, err = s.ScheduleByName(ctx, "jobstest.tick")
	if err != nil || sched.LastRun == nil || sched.LastRun.ID != job.ID || sched.LastRun.State != "completed" {
		t.Fatalf("last run: %+v %v", sched, err)
	}
	if _, err := s.RunNow(ctx, "jobstest.none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("run unknown: %v", err)
	}

	got, err := s.Get(ctx, job.ID)
	if err != nil || got.State != "completed" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.Get(ctx, 1<<62); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get unknown: %v", err)
	}
	later, err := s.Enqueue(ctx, tickArgs{}, &river.InsertOpts{Queue: queueFast, ScheduledAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled, err := s.Cancel(ctx, later.ID); err != nil || cancelled.State != "cancelled" {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	if retried, err := s.Retry(ctx, later.ID); err != nil || retried.State != "available" {
		t.Fatalf("retry: %+v %v", retried, err)
	}
}

// Event retention deletes only what is older than the retention. The two events live in a
// transaction that is never committed: other test packages delete events by position to
// simulate retention (events, httpapi) and cannot see them there.
func TestEventRetention(t *testing.T) {
	ctx, pool := setup(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := queries.New(tx)
	id := uuid.New()
	old, err := q.InsertEvent(ctx, queries.InsertEventParams{Kind: "test", TypeName: "jobstest.thing", RecordID: id, Payload: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := q.InsertEvent(ctx, queries.InsertEventParams{Kind: "test", TypeName: "jobstest.thing", RecordID: id, Payload: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE event SET at = now() - interval '8 days' WHERE position = $1", old.Position); err != nil {
		t.Fatal(err)
	}
	w := &eventRetentionWorker{dbs: db.Single(pool), keep: 7 * 24 * time.Hour, logger: quiet, db: tx}
	// A durable consumer that has not taken the old event holds it back (PL41).
	if _, err := tx.Exec(ctx, `INSERT INTO event_consumer (name, position, durable) VALUES ('jobstest-slow', $1, true)
		ON CONFLICT (name) DO UPDATE SET position = $1, durable = true`, old.Position-1); err != nil {
		t.Fatal(err)
	}
	if err := w.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	var held bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM event WHERE position = $1)", old.Position).Scan(&held); err != nil || !held {
		t.Fatalf("a consumer behind the old event must hold it back: %v %v", held, err)
	}
	// Once every durable consumer has taken it, it goes (the transaction is rolled back).
	if _, err := tx.Exec(ctx, "UPDATE event_consumer SET position = greatest(position, $1) WHERE durable", fresh.Position); err != nil {
		t.Fatal(err)
	}
	if err := w.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	// Read by position: ListEvents hides the rows of an open transaction (horizon, E87).
	kept := map[int64]bool{}
	rows, err := tx.Query(ctx, "SELECT position FROM event WHERE position = ANY($1)", []int64{old.Position, fresh.Position})
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var pos int64
		if err := rows.Scan(&pos); err != nil {
			t.Fatal(err)
		}
		kept[pos] = true
	}
	rows.Close()
	if kept[old.Position] {
		t.Fatal("the old event must be gone")
	}
	if !kept[fresh.Position] {
		t.Fatal("the fresh event must stay")
	}
	s := New(db.Single(pool), Options{Logger: quiet})
	if err := RegisterEventRetention(s, 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	sched, err := s.ScheduleByName(ctx, "event.retention")
	if err != nil || sched.Schedule != "30 2 * * *" || sched.Queue != QueueDefault {
		t.Fatalf("retention schedule: %+v %v", sched, err)
	}
}
