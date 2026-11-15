// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package sla runs deadlines and SLA clocks (WF9; Hauptplan KF5; architektur.md §8). An SLA
// is a record of the core type core.sla: a target in working minutes for the records of one
// type, with CEL conditions for when it holds, pauses and is met. Its clocks live in the core
// table clock and move in the transaction of the write that moves them (records.Clocks); a
// River job waits for the deadline of every running clock, there is no run over all clocks.
package sla

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/memo"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules"
)

// TypeSLA is the core type of the SLAs.
const TypeSLA = "core.sla"

// reloadAfter is how long the SLAs of an instance stay without a word from the others.
const reloadAfter = 30 * time.Second

// channel tells the other instances that SLAs changed.
const channel = "itsm:sla:changed"

// SLA is a service level as the clocks use it.
type SLA struct {
	ID       uuid.UUID
	Name     string
	TypeName string
	Target   time.Duration
	Calendar uuid.UUID // uuid.Nil: the calendar of the record's unit
	// FromApplies counts a clock from the moment the SLA comes to hold, not from the creation
	// of the record (start_from applies, WF11: a wait starts anew each time).
	FromApplies bool
	// RoundTheClock counts every minute, whatever calendar the unit has (counts
	// round_the_clock, WF12).
	RoundTheClock bool
	// TargetField names a number field of the record: the target is Target times its value,
	// the delivery time of an offer in working days for example (SD13, SD-Plan N20).
	TargetField string
	// TargetSetting names a setting of the organisation tree (SD20): its value at the
	// record's unit, in minutes, is the target instead of Target; 0 means no clock.
	TargetSetting string
	applies       *rules.Condition
	pause         *rules.Condition
	stop          *rules.Condition
}

// Service keeps the SLAs and moves the clocks.
type Service struct {
	db     *db.Router
	rec    *records.Service
	cals   *calendar.Service
	jobs   *jobs.Service
	kv     *cache.Client
	logger *slog.Logger
	// Queue is the River queue of the deadline jobs; tests use their own.
	Queue string
	// Now is the time of the clocks; tests set it.
	Now func() time.Time
	// Levels returns the escalation levels of an SLA for a type, lowest first (WF10: the rules
	// with trigger clock); nil means none.
	Levels func(ctx context.Context, typeName string, sla uuid.UUID) ([]Level, error)
	// Settings reads a setting of a node with the default of its declaration (SD20: the
	// target of an SLA from a setting); nil means the targets of the SLAs.
	Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)
	// Declared says how a setting is declared, for checking target_setting; nil checks nothing.
	Declared func(ctx context.Context, key string) (meta.Setting, bool, error)

	// slas are the active SLAs in memory (PL58): a request takes the last state, a reload runs
	// in the background, never in a request that holds a transaction.
	slas memo.Value[slaSet]
	mu   sync.RWMutex
	// calendars are the calendars of the clocks in memory, by database and id (PL53); an old
	// one serves while it is read anew in the background (PL58).
	calendars map[string]*calendarEntry
}

// slaSet are the active SLAs by type and by id.
type slaSet struct {
	byType map[string][]*SLA
	byID   map[uuid.UUID]*SLA
}

// calendarEntry is a calendar in memory, when it was loaded and whether a reload runs.
type calendarEntry struct {
	c       Counter
	at      time.Time
	loading bool
}

// calendarMaxAge is the longest a calendar stays in memory (PL53, like the settings, SD20); a
// change of a calendar drops them at once here and on the other instances (Wrote).
const calendarMaxAge = 10 * time.Second

// New wires the service; kv may be nil (then only this instance hears of changes).
func New(rt *db.Router, rec *records.Service, cals *calendar.Service, js *jobs.Service, kv *cache.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{db: rt, rec: rec, cals: cals, jobs: js, kv: kv, logger: logger, Queue: jobs.QueueDefault, Now: time.Now}
	s.slas = memo.Value[slaSet]{MaxAge: reloadAfter, Load: s.read, Logger: logger, Name: "sla"}
	return s
}

// Warm loads the SLAs before the first request (PL58): the first load is the only one that
// runs in a caller.
func (s *Service) Warm(ctx context.Context) error {
	_, err := s.slas.Refresh(ctx)
	return err
}

// Run keeps the SLAs in step with the other instances until the context ends.
func (s *Service) Run(ctx context.Context) error {
	if s.kv == nil {
		<-ctx.Done()
		return nil
	}
	for ctx.Err() == nil {
		err := s.kv.Subscribe(ctx, channel, func(string) { s.stale() })
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("sla: subscription lost, retrying", "error", err.Error())
			time.Sleep(time.Second)
		}
	}
	return nil
}

// stale marks what is in memory as old after a change on another instance: it is read anew in
// the background (PL58).
func (s *Service) stale() {
	s.slas.Stale()
	s.mu.Lock()
	for _, e := range s.calendars {
		e.at = time.Time{}
	}
	s.mu.Unlock()
}

// Wrote is told of every write (records.AfterWrite): a change of an SLA drops them here and on
// the other instances.
func (s *Service) Wrote(ctx context.Context, typeName string) {
	if typeName != TypeSLA && !strings.HasPrefix(typeName, "core.calendar") {
		return
	}
	// The own change counts for the next request of this instance: read anew here, after the
	// commit of the write, and the calendars at their next use.
	if _, err := s.slas.Refresh(context.WithoutCancel(ctx)); err != nil {
		s.logger.Warn("sla: reload after a change failed", "error", err.Error())
		s.slas.Forget()
	}
	s.mu.Lock()
	s.calendars = nil
	s.mu.Unlock()
	if s.kv != nil {
		if err := s.kv.Publish(ctx, channel, "1"); err != nil {
			s.logger.Warn("sla: publish", "error", err.Error())
		}
	}
}

// system is the context of the clocks: they read SLAs, calendars and settings with the rights
// of the system, whoever wrote.
func system(ctx context.Context) context.Context {
	return authz.WithAccess(ctx, authz.System())
}

// load returns the active SLAs by type and by id: the last state in memory (PL58).
func (s *Service) load(ctx context.Context) (map[string][]*SLA, map[uuid.UUID]*SLA, error) {
	set, err := s.slas.Get(ctx)
	if err != nil {
		return nil, nil, err
	}
	return set.byType, set.byID, nil
}

// read reads the active SLAs from the database.
func (s *Service) read(ctx context.Context) (slaSet, error) {
	byType, byID := map[string][]*SLA{}, map[uuid.UUID]*SLA{}
	if _, err := s.rec.Type(ctx, TypeSLA); errors.Is(err, records.ErrNotFound) {
		// The core package of an instance not yet updated has no SLAs.
		return slaSet{byType, byID}, nil
	}
	after := ""
	for {
		page, err := s.rec.List(system(ctx), TypeSLA, records.ListOptions{Limit: 500, After: after})
		if err != nil {
			return slaSet{}, fmt.Errorf("sla: load: %w", err)
		}
		for _, r := range page.Items {
			if active, _ := r["active"].(bool); !active {
				continue
			}
			sla, err := fromRecord(r)
			if err != nil {
				s.logger.Warn("sla: unusable, left out", "sla", r["name"], "error", err.Error())
				continue
			}
			byType[sla.TypeName] = append(byType[sla.TypeName], sla)
			byID[sla.ID] = sla
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	return slaSet{byType, byID}, nil
}

// fromRecord reads an SLA from its record and compiles its conditions.
func fromRecord(r records.Record) (*SLA, error) {
	id, err := uuid.Parse(fmt.Sprint(r["id"]))
	if err != nil {
		return nil, err
	}
	minutes, err := strconv.ParseFloat(fmt.Sprint(r["target_minutes"]), 64)
	if err != nil || minutes <= 0 {
		return nil, fmt.Errorf("target_minutes %v", r["target_minutes"])
	}
	sla := &SLA{ID: id, Name: fmt.Sprint(r["name"]), TypeName: fmt.Sprint(r["target_type"]),
		Target: time.Duration(minutes * float64(time.Minute)).Round(time.Second), FromApplies: r["start_from"] == "applies",
		RoundTheClock: r["counts"] == "round_the_clock"}
	sla.TargetField, _ = r["target_field"].(string)
	sla.TargetSetting, _ = r["target_setting"].(string)
	if c, ok := r["calendar"].(string); ok && c != "" {
		if sla.Calendar, err = uuid.Parse(c); err != nil {
			return nil, err
		}
	}
	for _, x := range []struct {
		field string
		to    **rules.Condition
	}{{"applies", &sla.applies}, {"pause", &sla.pause}, {"stop", &sla.stop}} {
		expr, _ := r[x.field].(string)
		if expr == "" {
			continue
		}
		if *x.to, err = rules.Compile(expr); err != nil {
			return nil, fmt.Errorf("%s: %w", x.field, err)
		}
	}
	if sla.stop == nil {
		return nil, errors.New("stop is empty")
	}
	return sla, nil
}

// Check is the RecordCheck of records: an SLA must name a type that exists, and its
// conditions must compile.
func (s *Service) Check(ctx context.Context, t meta.Type, rec map[string]any) []records.Problem {
	if t.QualifiedName() != TypeSLA {
		return nil
	}
	var ps []records.Problem
	if name, _ := rec["target_type"].(string); name != "" {
		if _, err := s.rec.Type(ctx, name); err != nil {
			ps = append(ps, records.Problem{Field: "target_type", Code: "unknown", Message: fmt.Sprintf("type %s is not installed", name)})
		}
	}
	for _, field := range []string{"applies", "pause", "stop"} {
		expr, _ := rec[field].(string)
		if expr == "" {
			continue
		}
		if _, err := rules.Compile(expr); err != nil {
			ps = append(ps, records.Problem{Field: field, Code: "condition", Message: err.Error()})
		}
	}
	// The target from a field (SD-Plan N20): a number field of the type.
	if name, _ := rec["target_field"].(string); name != "" {
		typeName, _ := rec["target_type"].(string)
		if target, err := s.rec.Type(ctx, typeName); err == nil {
			if f, ok := target.Field(name); !ok || f.Type != meta.FieldNumber {
				ps = append(ps, records.Problem{Field: "target_field", Code: "invalid", Message: fmt.Sprintf("%s is no number field of %s", name, typeName)})
			}
		}
	}
	// The target from a setting (SD20): a declared setting of minutes.
	if key, _ := rec["target_setting"].(string); key != "" && s.Declared != nil {
		st, ok, err := s.Declared(ctx, key)
		switch {
		case err != nil:
			ps = append(ps, records.Problem{Field: "target_setting", Code: "invalid", Message: err.Error()})
		case !ok:
			ps = append(ps, records.Problem{Field: "target_setting", Code: "unknown", Message: fmt.Sprintf("no package declares the setting %s", key)})
		case st.Kind != meta.SettingDuration && st.Kind != meta.SettingNumber:
			ps = append(ps, records.Problem{Field: "target_setting", Code: "invalid", Message: fmt.Sprintf("the setting %s is a %s, not minutes", key, st.Kind)})
		}
	}
	return ps
}

// targetOf is the target of a new clock: the SLA's, times the number in its target field when
// the record has one above zero (SD-Plan N20).
func targetOf(sla *SLA, base time.Duration, rec records.Record) time.Duration {
	if sla.TargetField == "" {
		return base
	}
	var n float64
	switch v := rec[sla.TargetField].(type) {
	case float64:
		n = v
	case int64:
		n = float64(v)
	case int:
		n = float64(v)
	case string:
		n, _ = strconv.ParseFloat(v, 64)
	}
	if n <= 0 {
		return base
	}
	return time.Duration(float64(base) * n).Round(time.Second)
}

// target is the target of a new clock: the setting the SLA names at the record's unit (SD20),
// else the SLA's own, times the number field of the record (SD13). false when the setting is
// 0: then the SLA starts no clock there, such as "do not close by itself". A setting that is no
// number keeps the SLA's own target.
func (s *Service) target(ctx context.Context, sla *SLA, rec records.Record) (time.Duration, bool) {
	base := sla.Target
	if sla.TargetSetting != "" && s.Settings != nil {
		path, _ := rec["org_path"].(string)
		if path == "" {
			path = "root"
		}
		raw, _, err := s.Settings(ctx, path, sla.TargetSetting)
		var minutes float64
		switch {
		case err != nil:
			s.logger.Warn("sla: setting of the target not readable, using the SLA's", "sla", sla.Name, "setting", sla.TargetSetting, "error", err.Error())
		case raw == nil || json.Unmarshal(raw, &minutes) != nil || minutes < 0:
		case minutes == 0:
			return 0, false
		default:
			base = time.Duration(minutes * float64(time.Minute)).Round(time.Second)
		}
	}
	return targetOf(sla, base, rec), true
}

// Times tells records whether a type has SLAs (records.Clocks). When they cannot be read, it
// says yes, so that Written refuses the write with the error instead of skipping the clocks.
func (s *Service) Times(ctx context.Context, typeName string) bool {
	byType, _, err := s.load(ctx)
	if err != nil {
		s.logger.Error("sla: SLAs not readable", "error", err.Error())
		return true
	}
	return len(byType[typeName]) > 0
}

// holds evaluates a condition; one that cannot decide (a missing field) is false and logged.
func (s *Service) holds(sla *SLA, what string, c *rules.Condition, rec records.Record) bool {
	ok, err := c.Eval(rec)
	if err != nil {
		s.logger.Warn("sla: condition failed", "sla", sla.Name, "condition", what, "error", err.Error())
		return false
	}
	return ok
}

// wanted is the state the clock of an SLA should be in for the record as it is, and whether
// the SLA holds at all. In the trash the clocks hold (WF-Plan E10).
func (s *Service) wanted(sla *SLA, rec records.Record) (string, bool) {
	if sla.applies != nil && !s.holds(sla, "applies", sla.applies, rec) {
		return StateCancelled, false
	}
	switch {
	case rec["deleted_at"] != nil:
		return StatePaused, true
	case s.holds(sla, "stop", sla.stop, rec):
		return StateStopped, true
	case sla.pause != nil && s.holds(sla, "pause", sla.pause, rec):
		return StatePaused, true
	}
	return StateRunning, true
}

// Written moves the clocks of a record in the transaction of its write (records.Clocks):
// a clock starts when its SLA comes to hold, pauses, goes on or stops as the record says, and
// is cancelled when the SLA no longer holds; a purge takes them away. A clock that went past
// its target leaves a breach at the record; a running one gets a job for its deadline.
func (s *Service) Written(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec records.Record) error {
	q := queries.New(tx)
	if rec == nil {
		return q.DeleteClocksOfRecord(ctx, id)
	}
	byType, _, err := s.load(ctx)
	if err != nil {
		return err
	}
	list := byType[t.QualifiedName()]
	if len(list) == 0 {
		return nil
	}
	rows, err := q.ClocksOfRecordForUpdate(ctx, id)
	if err != nil {
		return fmt.Errorf("sla: clocks: %w", err)
	}
	have := map[uuid.UUID]queries.Clock{}
	for _, r := range rows {
		have[r.Sla] = r
	}
	now := s.Now()
	for _, sla := range list {
		state, holds := s.wanted(sla, rec)
		row, exists := have[sla.ID]
		var c *Clock
		var n Counter
		var breached bool
		switch {
		case exists && row.State != StateCancelled:
			c = fromRow(row)
			if n, err = s.counter(ctx, c.Calendar); err != nil {
				return err
			}
			if breached, err = c.move(state, now, n); err != nil {
				return err
			}
			if c.Generation == row.Generation {
				continue
			}
		case !holds || state == StateStopped:
			// No clock for an SLA that does not hold, or is met before it ever ran.
			continue
		default:
			// A new clock, or one cancelled before whose SLA holds again: it counts from the
			// creation of the record (on the first write that is now), or from now when the
			// SLA counts from the moment it holds (WF11).
			target, ok := s.target(ctx, sla, rec)
			if !ok {
				continue
			}
			c = &Clock{ID: uuid.New(), TypeName: t.QualifiedName(), RecordID: id, SLA: sla.ID, Target: target, StartedAt: now}
			if exists {
				c.ID, c.Generation = row.ID, row.Generation
			}
			if v, _ := rec["version"].(int64); v > 1 && !sla.FromApplies {
				if at, ok := timeOf(rec["created_at"]); ok && at.Before(now) {
					c.StartedAt = at
				}
			}
			if c.Calendar, err = s.calendarFor(ctx, sla, rec); err != nil {
				return err
			}
			if n, err = s.counter(ctx, c.Calendar); err != nil {
				return err
			}
			if breached, err = start(c, state, now, n); err != nil {
				return err
			}
		}
		// Levels passed before the job came, or at once on a late start (WF10): Used holds all
		// the working time now, since a clock that runs has just begun its run.
		if err := s.settle(ctx, tx, q, sla, c, exists, n, c.Used, breached); err != nil {
			return err
		}
	}
	return s.setDeadline(ctx, tx, q, t.QualifiedName(), id)
}

// setDeadline leaves the deadline of a record's clocks at the record for its list (UI34,
// UI-Plan E37): the running clock due first, with the moment it has used three quarters of its
// target on its own calendar; without one running, a paused clock marks the record paused.
func (s *Service) setDeadline(ctx context.Context, tx pgx.Tx, q *queries.Queries, typeName string, id uuid.UUID) error {
	rows, err := q.ClocksOfRecord(ctx, id)
	if err != nil {
		return fmt.Errorf("sla: clocks: %w", err)
	}
	var d records.Deadline
	var first *Clock
	for _, r := range rows {
		c := fromRow(r)
		switch {
		case c.State == StateRunning && c.DueAt != nil && c.ResumedAt != nil:
			if first == nil || c.DueAt.Before(*first.DueAt) {
				first = c
			}
		case c.State == StatePaused:
			d.Paused = true
		}
	}
	if first != nil {
		n, err := s.counter(ctx, first.Calendar)
		if err != nil {
			return err
		}
		warn, err := n.Add(*first.ResumedAt, max(first.Target*3/4-first.Used, 0))
		if err != nil {
			return err
		}
		d = records.Deadline{Due: first.DueAt, Warn: &warn}
	}
	return s.rec.SetDeadlineTx(ctx, tx, typeName, id, d)
}

// settle notes the levels a clock reached with elapsed working time, saves it and plans its
// next job; levels and a breach leave their entries at the record, levels first.
func (s *Service) settle(ctx context.Context, tx pgx.Tx, q *queries.Queries, sla *SLA, c *Clock, exists bool, n Counter,
	elapsed time.Duration, breached bool) error {
	levels, err := s.levelsOf(ctx, c)
	if err != nil {
		return err
	}
	reached := c.reach(levels, elapsed)
	if err := s.save(ctx, tx, q, c, exists, levels, n); err != nil {
		return err
	}
	for _, l := range reached {
		payload := map[string]any{"sla": sla.ID.String(), "sla_name": sla.Name, "clock": c.ID.String(), "rule": l.Rule}
		if l.Minutes > 0 {
			payload["minutes"] = l.Minutes
		} else {
			payload["percent"] = l.Percent
		}
		if err := s.rec.TraceTx(ctx, tx, c.TypeName, c.RecordID, "escalation", payload); err != nil {
			return err
		}
	}
	if breached {
		return s.breach(ctx, tx, sla, c)
	}
	return nil
}

// levelsOf returns the escalation levels of a clock's SLA, the earliest first.
func (s *Service) levelsOf(ctx context.Context, c *Clock) ([]Level, error) {
	if s.Levels == nil {
		return nil, nil
	}
	list, err := s.Levels(ctx, c.TypeName, c.SLA)
	sort.SliceStable(list, func(i, j int) bool { return list[i].at(c.Target) < list[j].at(c.Target) })
	return list, err
}

// save writes a clock and, while it runs, the job for its next level or its deadline.
func (s *Service) save(ctx context.Context, tx pgx.Tx, q *queries.Queries, c *Clock, exists bool, levels []Level, n Counter) error {
	if c.Levels == nil {
		c.Levels = []string{}
	}
	if exists {
		err := q.UpdateClock(ctx, queries.UpdateClockParams{ID: c.ID, State: c.State, TargetSeconds: seconds(c.Target),
			Calendar: nullUUID(c.Calendar), StartedAt: ts(&c.StartedAt), UsedSeconds: seconds(c.Used), ResumedAt: ts(c.ResumedAt),
			DueAt: ts(c.DueAt), BreachedAt: ts(c.BreachedAt), StoppedAt: ts(c.StoppedAt), Generation: c.Generation, Levels: c.Levels})
		if err != nil {
			return fmt.Errorf("sla: save clock: %w", err)
		}
	} else {
		err := q.InsertClock(ctx, queries.InsertClockParams{ID: c.ID, TypeName: c.TypeName, RecordID: c.RecordID, Sla: c.SLA,
			State: c.State, TargetSeconds: seconds(c.Target), Calendar: nullUUID(c.Calendar), StartedAt: ts(&c.StartedAt),
			UsedSeconds: seconds(c.Used), ResumedAt: ts(c.ResumedAt), DueAt: ts(c.DueAt), BreachedAt: ts(c.BreachedAt),
			StoppedAt: ts(c.StoppedAt), Generation: c.Generation, Levels: c.Levels})
		if err != nil {
			return fmt.Errorf("sla: new clock: %w", err)
		}
	}
	at, ok, err := c.next(levels, n)
	if err != nil || !ok {
		return err
	}
	_, err = s.jobs.EnqueueTx(ctx, tx, DueArgs{Clock: c.ID, Generation: c.Generation},
		&river.InsertOpts{Queue: s.Queue, ScheduledAt: at})
	return err
}

// breach notes at the record that a clock went past its target: an audit row and the event
// "breach" with the SLA and the moment, for rules, webhooks and the live view.
func (s *Service) breach(ctx context.Context, tx pgx.Tx, sla *SLA, c *Clock) error {
	payload := map[string]any{"sla": sla.ID.String(), "sla_name": sla.Name, "clock": c.ID.String(),
		"due_at": c.BreachedAt.UTC().Format(time.RFC3339)}
	return s.rec.TraceTx(ctx, tx, c.TypeName, c.RecordID, "breach", payload)
}

// calendarFor is the calendar a new clock counts by: none for an SLA round the clock, else the
// SLA's own, else the one the setting calendar.default gives the record's unit, else none.
func (s *Service) calendarFor(ctx context.Context, sla *SLA, rec records.Record) (uuid.UUID, error) {
	if sla.RoundTheClock {
		return uuid.Nil, nil
	}
	if sla.Calendar != uuid.Nil {
		return sla.Calendar, nil
	}
	path, _ := rec["org_path"].(string)
	if path == "" {
		path = "root"
	}
	_, id, err := s.cals.ForNode(system(ctx), path)
	if err != nil {
		s.logger.Warn("sla: calendar of the unit not usable, counting round the clock", "path", path, "error", err.Error())
		return uuid.Nil, nil
	}
	return id, nil
}

// counter loads the calendar of a clock. A calendar that cannot count (gone, broken, without
// working hours) counts round the clock and is logged: a broken calendar must not stop the
// writes of every ticket.
func (s *Service) counter(ctx context.Context, id uuid.UUID) (Counter, error) {
	if id == uuid.Nil {
		return RoundTheClock{}, nil
	}
	// In memory for a short while (PL53): loading a calendar read its hours and holidays for
	// every write of a record with a clock.
	key := fmt.Sprintf("%p/%s", s.db.Tenant(ctx), id)
	s.mu.Lock()
	e, ok := s.calendars[key]
	if ok {
		// An old calendar serves while it is read anew in the background (PL58).
		if time.Since(e.at) >= calendarMaxAge && !e.loading {
			e.loading = true
			go s.reloadCalendar(context.WithoutCancel(ctx), key, id)
		}
		c := e.c
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()
	c, _ := s.readCalendar(ctx, id)
	s.mu.Lock()
	if s.calendars == nil {
		s.calendars = map[string]*calendarEntry{}
	}
	s.calendars[key] = &calendarEntry{c: c, at: time.Now()}
	s.mu.Unlock()
	return c, nil
}

// readCalendar reads a calendar; one that cannot count counts round the clock, and ok is false.
func (s *Service) readCalendar(ctx context.Context, id uuid.UUID) (Counter, bool) {
	c, err := s.cals.Load(system(ctx), id)
	if err == nil {
		_, err = c.Add(s.Now(), time.Minute)
	}
	if err != nil {
		s.logger.Warn("sla: calendar not usable, counting round the clock", "calendar", id, "error", err.Error())
		return RoundTheClock{}, false
	}
	return c, true
}

func (s *Service) reloadCalendar(ctx context.Context, key string, id uuid.UUID) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, ok := s.readCalendar(ctx, id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, there := s.calendars[key]; there {
		// A failed reload keeps the calendar that counted until now; the next use tries again.
		if ok {
			e.c, e.at = c, time.Now()
		}
		e.loading = false
	}
}

// --- conversions ---

func fromRow(r queries.Clock) *Clock {
	c := &Clock{ID: r.ID, TypeName: r.TypeName, RecordID: r.RecordID, SLA: r.Sla, State: r.State,
		Target: time.Duration(r.TargetSeconds) * time.Second, StartedAt: r.StartedAt.Time,
		Used: time.Duration(r.UsedSeconds) * time.Second, ResumedAt: timeAt(r.ResumedAt), DueAt: timeAt(r.DueAt),
		BreachedAt: timeAt(r.BreachedAt), StoppedAt: timeAt(r.StoppedAt), Generation: r.Generation, Levels: r.Levels}
	if r.Calendar.Valid {
		c.Calendar = r.Calendar.UUID
	}
	return c
}

func seconds(d time.Duration) int64 { return int64(d / time.Second) }

func ts(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timeAt(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func nullUUID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

// timeOf reads a time of a record: a time.Time or an RFC 3339 string.
func timeOf(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case string:
		t, err := time.Parse(time.RFC3339Nano, x)
		return t, err == nil
	}
	return time.Time{}, false
}
