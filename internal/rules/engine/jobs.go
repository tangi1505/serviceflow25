// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/robfig/cron/v3"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// --- rules after the write and on a schedule (WF4) ---

// ApplyArgs is the job of the rules after one write: the event that told of it.
type ApplyArgs struct {
	Type     string    `json:"type"`
	Record   uuid.UUID `json:"record"`
	Position int64     `json:"position"`
	Trigger  string    `json:"trigger"`
	Changed  []string  `json:"changed"`
	// Depth is how deep the write lies in a chain of rules; By is the rule that made it.
	Depth int    `json:"depth"`
	By    string `json:"by,omitempty"`
	// Rule, when set, is the one rule to run: the escalation level a clock reached (WF10).
	Rule string `json:"rule,omitempty"`
}

// Kind names the job.
func (ApplyArgs) Kind() string { return "rules.apply" }

// TimeArgs is the job that runs the rules on a schedule, every minute.
type TimeArgs struct{}

// Kind names the job.
func (TimeArgs) Kind() string { return "rules.time" }

// Register adds the workers and the minute job that looks after the schedules.
func (e *Engine) Register(js *jobs.Service) error {
	e.RegisterWorkers(js)
	return js.AddPeriodic(jobs.Periodic{
		Name:     TimeArgs{}.Kind(),
		Label:    map[string]string{"de": "Regeln nach Zeitplan", "en": "Rules on a schedule"},
		Schedule: "* * * * *",
		Queue:    jobs.QueueDefault,
		Args:     func() river.JobArgs { return TimeArgs{} },
	})
}

// HandleBatch is the consumer rules of the dispatcher (PL41): every event of a write whose type
// has rules after it gets one job, all jobs of the batch in one statement of the consumer's
// transaction, so each event leads to its job exactly once. A write at MaxDepth of a chain
// gets none; the log says so.
func (e *Engine) HandleBatch(ctx context.Context, tx pgx.Tx, batch []records.Event) error {
	var params []river.InsertManyParams
	for _, ev := range batch {
		args, ok, err := e.argsOf(ctx, ev)
		if err != nil {
			return err
		}
		if ok {
			params = append(params, river.InsertManyParams{Args: args, InsertOpts: &river.InsertOpts{Queue: e.Queue}})
		}
	}
	return e.jobs.EnqueueManyTx(ctx, tx, params)
}

// Handle runs one event as a batch of its own, in a transaction of its own (tests).
func (e *Engine) Handle(ctx context.Context, ev records.Event) error {
	return pgx.BeginFunc(ctx, e.db.Tenant(ctx), func(tx pgx.Tx) error {
		return e.HandleBatch(ctx, tx, []records.Event{ev})
	})
}

// argsOf is the job of the rules after one event, if its type has any for the trigger.
func (e *Engine) argsOf(ctx context.Context, ev records.Event) (ApplyArgs, bool, error) {
	trigger := ""
	switch ev.Kind {
	case "create":
		trigger = TriggerCreate
	case "update", "transition":
		trigger = TriggerUpdate
	case "escalation":
		return e.levelArgs(ctx, ev)
	default:
		return ApplyArgs{}, false, nil
	}
	list, err := e.of(ctx, ev.TypeName, trigger, ModeAfter)
	if err != nil {
		return ApplyArgs{}, false, err
	}
	if len(only(list, records.EventPath(ev))) == 0 {
		return ApplyArgs{}, false, nil
	}
	var payload struct {
		Changed []string `json:"changed"`
		Rule    string   `json:"rule"`
		Depth   int      `json:"depth"`
	}
	_ = json.Unmarshal(ev.Payload, &payload)
	if payload.Depth >= MaxDepth {
		e.logger.Warn("rules: chain too deep, stopped", "type", ev.TypeName, "record", ev.RecordID, "rule", payload.Rule, "depth", payload.Depth)
		return ApplyArgs{}, false, nil
	}
	return ApplyArgs{Type: ev.TypeName, Record: ev.RecordID, Position: ev.Position, Trigger: trigger,
		Changed: payload.Changed, Depth: payload.Depth, By: payload.Rule}, true, nil
}

// levelArgs is the job of the escalation level a clock reached (WF10): the event names the
// rule; a level switched off or removed meanwhile gets none.
func (e *Engine) levelArgs(ctx context.Context, ev records.Event) (ApplyArgs, bool, error) {
	var payload struct {
		Rule string `json:"rule"`
	}
	_ = json.Unmarshal(ev.Payload, &payload)
	list, err := e.of(ctx, ev.TypeName, TriggerClock, ModeAfter)
	if err != nil {
		return ApplyArgs{}, false, err
	}
	for _, c := range only(list, records.EventPath(ev)) {
		if c.Name == payload.Rule {
			return ApplyArgs{Type: ev.TypeName, Record: ev.RecordID, Position: ev.Position, Trigger: TriggerClock,
				Changed: []string{}, Rule: c.Name}, true, nil
		}
	}
	return ApplyArgs{}, false, nil
}

// RegisterWorkers adds the workers only; tests run the schedules themselves.
func (e *Engine) RegisterWorkers(js *jobs.Service) {
	jobs.AddWorker(js, &applyWorker{e: e})
	jobs.AddWorker(js, &timeWorker{e: e})
}

type applyWorker struct {
	river.WorkerDefaults[ApplyArgs]
	e *Engine
}

// Work runs the rules after a write; what fails on the last attempt, or is given up, is kept
// in the failures of the dispatcher (PL41).
func (w *applyWorker) Work(ctx context.Context, job *river.Job[ApplyArgs]) error {
	rule, err := w.run(ctx, job)
	var cancel *rivertype.JobCancelError
	if err != nil && (job.Attempt >= job.MaxAttempts || errors.As(err, &cancel)) {
		pos, id, rec := job.Args.Position, job.ID, job.Args.Record
		if rule == "" {
			rule = "(" + job.Args.Type + ")"
		}
		f := events.Failure{Kind: events.FailureRule, Target: rule, EventPosition: &pos, TypeName: job.Args.Type, RecordID: &rec,
			JobID: &id, Attempts: job.Attempt, Error: err.Error()}
		if ferr := events.RecordFailure(ctx, w.e.db.Tenant(ctx), f); ferr != nil {
			w.e.logger.Error("rules: failure not kept", "error", ferr.Error())
		}
	}
	return err
}

// run runs the rules after a write against the record as it is now, in order; each match is
// a write of its own. A rule does not fire on its own write. It returns the rule it was at.
func (w *applyWorker) run(ctx context.Context, job *river.Job[ApplyArgs]) (string, error) {
	a := job.Args
	list, err := w.e.of(ctx, a.Type, a.Trigger, ModeAfter)
	if err != nil {
		return "", err
	}
	var fired []string
	for _, c := range list {
		if c.Name == a.By || (a.Rule != "" && c.Name != a.Rule) {
			continue
		}
		rec, err := w.e.rec.Get(ctx, a.Type, a.Record)
		if errors.Is(err, records.ErrNotFound) {
			break
		}
		if err != nil {
			return c.Name, err
		}
		if path, _ := rec["org_path"].(string); !c.reaches(path) || !w.e.matches(c, rec, a.Changed) {
			continue
		}
		if err := w.e.apply(ctx, c.Rule, a.Type, a.Record, a.Depth+1); err != nil {
			return c.Name, err
		}
		fired = append(fired, c.Name)
		if c.Stop {
			break
		}
	}
	return "", river.RecordOutput(ctx, map[string]any{"fired": fired})
}

// apply carries out the actions of a rule on a record: set and assign as one change, then
// the records to create, the notifications and the transitions in their order. The writes
// name the rule and the depth.
// A write the record refuses (a value no longer valid) cancels the job instead of retrying.
func (e *Engine) apply(ctx context.Context, r Rule, typeName string, id uuid.UUID, depth int) error {
	ctx = records.WithRule(ctx, r.Name, depth)
	rec, err := e.rec.Get(ctx, typeName, id)
	if err != nil {
		return err
	}
	set, roles := e.changes(r, rec)
	patch := map[string]any{}
	for k, v := range set {
		patch[k] = v
	}
	if len(roles) > 0 {
		patch["actors"] = roles
	}
	if len(patch) > 0 {
		if _, err := e.rec.Update(ctx, typeName, id, -1, patch, nil); err != nil {
			return refused(r, err)
		}
	}
	for _, a := range r.Actions {
		switch {
		case a.Create != nil:
			fields, err := e.inTenant(ctx, r, rec, a.Create.Fields)
			if err != nil {
				return err
			}
			if _, err := e.rec.Create(ctx, a.Create.TypeName, fields, nil); err != nil {
				return refused(r, err)
			}
		case a.Notify != nil:
			if rec, err = e.rec.Get(ctx, typeName, id); err != nil {
				return err
			}
			to := []Ref{}
			for _, target := range a.Notify.To {
				if spec, ok := strings.CutPrefix(target, "actors."); ok {
					role, lead := strings.CutSuffix(spec, meta.LeadSuffix)
					for _, h := range holdersOf(rec, role) {
						m, _ := h.(map[string]any)
						typ, _ := m["type"].(string)
						hid, _ := uuid.Parse(fmt.Sprint(m["id"]))
						if !lead {
							to = append(to, Ref{Type: typ, ID: hid})
							continue
						}
						// The responsible person of a group that holds the role (SD-Plan N10).
						if typ != "core.group" {
							continue
						}
						g, err := e.rec.Get(ctx, "core.group", hid)
						if errors.Is(err, records.ErrNotFound) {
							continue
						}
						if err != nil {
							return err
						}
						if pid, err := uuid.Parse(fmt.Sprint(g["responsible"])); err == nil {
							to = append(to, Ref{Type: "core.person", ID: pid})
						}
					}
					continue
				}
				typ, raw, _ := strings.Cut(target, ":")
				if hid, err := uuid.Parse(raw); err == nil {
					to = append(to, Ref{Type: typ, ID: hid})
				}
			}
			payload := map[string]any{"rule": r.Name, "message": a.Notify.Message, "to": to}
			if err := e.rec.Trace(ctx, typeName, id, "notify", payload, nil); err != nil {
				return err
			}
		case a.Transition != nil:
			// A transition the record no longer allows (it moved on meanwhile) is refused and
			// kept with the failures.
			move := ctx
			if a.Transition.Comment != "" {
				move = records.WithComment(ctx, a.Transition.Comment)
			}
			if _, err := e.rec.Transition(move, typeName, id, a.Transition.Name, a.Transition.Fields, -1, nil); err != nil {
				return refused(r, err)
			}
		case a.Related != nil:
			if err := e.related(ctx, r, typeName, id, a.Related); err != nil {
				return err
			}
		}
	}
	return nil
}

// related moves the records linked by the kind along their transition (SD5). The values
// copied are read after the actions before, so a rule that sets the solution and then solves
// the linked records hands on the new one.
func (e *Engine) related(ctx context.Context, r Rule, typeName string, id uuid.UUID, a *Related) error {
	rec, err := e.rec.Get(ctx, typeName, id)
	if err != nil {
		return err
	}
	rows, err := queries.New(e.db.Tenant(ctx)).ListRelationsOf(ctx, queries.ListRelationsOfParams{TypeName: typeName, RecordID: id,
		Outgoing: a.Direction == "out", Incoming: a.Direction == "in", Kind: pgtype.Text{String: a.Kind, Valid: true}})
	if err != nil {
		return fmt.Errorf("rules: relations of %s/%s: %w", typeName, id, err)
	}
	fields := map[string]any{}
	for _, name := range a.Copy {
		fields[name] = rec[name]
	}
	for k, v := range a.Fields {
		fields[k] = v
	}
	move := ctx
	if a.Comment != "" {
		move = records.WithComment(ctx, a.Comment)
	}
	for _, row := range rows {
		otherType, otherID := row.FromType, row.FromID
		if a.Direction == "out" {
			otherType, otherID = row.ToType, row.ToID
		}
		t, err := e.rec.Type(ctx, otherType)
		if err != nil {
			return err
		}
		tr, ok := t.Transition(a.Transition)
		if !ok {
			continue
		}
		other, err := e.rec.Get(ctx, otherType, otherID)
		if errors.Is(err, records.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if !slices.Contains(tr.From, fmt.Sprint(other["state"])) {
			continue
		}
		if _, err := e.rec.Transition(move, otherType, otherID, a.Transition, fields, -1, nil); err != nil {
			return refused(r, err)
		}
	}
	return nil
}

// inTenant gives a record a rule creates the tenant of the rule, for a rule of the
// installation the tenant of the record it acts on; a node the rule names stays (PL45).
func (e *Engine) inTenant(ctx context.Context, r Rule, rec map[string]any, fields map[string]any) (map[string]any, error) {
	if _, ok := fields["org_path"]; ok {
		return fields, nil
	}
	node := r.Tenant
	if node == "" {
		tenants, err := org.TenantPaths(ctx, queries.New(e.db.Control()))
		if err != nil {
			return nil, err
		}
		path, _ := rec["org_path"].(string)
		node = org.TenantOf(tenants, path)
	}
	out := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		out[k] = v
	}
	out["org_path"] = node
	return out, nil
}

// refused cancels the job for a write the record refuses: retrying does not help, the log
// says which rule to fix.
func refused(r Rule, err error) error {
	if _, ok := records.AsValidationError(err); ok || errors.Is(err, records.ErrConflict) {
		return river.JobCancel(fmt.Errorf("rule %s: %w", r.Name, err))
	}
	return err
}

type timeWorker struct {
	river.WorkerDefaults[TimeArgs]
	e *Engine
}

// Work runs the rules on a schedule.
func (w *timeWorker) Work(ctx context.Context, _ *river.Job[TimeArgs]) error {
	ran, err := w.e.RunSchedules(ctx, time.Now())
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"ran": ran})
}

// RunSchedules runs every rule on a schedule whose next time has come since its last run (or
// its creation): the records its condition finds, at most MaxMatches, get its actions. It
// returns how many records each rule that ran found. It runs the rules kept in the database of
// the job (PL47c): a tenant's rules there; a rule of the installation lies in the control
// database and finds its records in every database, so it runs once and counts as run once.
func (e *Engine) RunSchedules(ctx context.Context, now time.Time) (map[string]int, error) {
	rows, err := queries.New(e.db.Tenant(ctx)).ListRules(ctx)
	if err != nil {
		return nil, err
	}
	ran := map[string]int{}
	for _, row := range rows {
		if !row.Enabled || row.Trigger != TriggerTime {
			continue
		}
		sched, err := cron.ParseStandard(row.Schedule)
		if err != nil {
			continue
		}
		since := row.CreatedAt.Time
		if row.LastRun.Valid {
			since = row.LastRun.Time
		}
		if sched.Next(since).After(now) {
			continue
		}
		r, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		where := []context.Context{ctx}
		if !row.Tenant.Valid && e.db.Separate() {
			where = where[:0]
			for _, d := range e.db.Databases() {
				where = append(where, d.Bind(ctx))
			}
		}
		found := 0
		for _, in := range where {
			ids, err := e.rec.Matching(in, r.TypeName, r.Condition, r.Tenant, MaxMatches)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				if err := e.apply(in, r, r.TypeName, id, 1); err != nil {
					e.logger.Warn("rules: scheduled rule failed on a record", "rule", r.Name, "record", id, "error", err.Error())
				}
			}
			found += len(ids)
		}
		if err := e.ruleDB(row.Tenant).SetRuleLastRun(ctx, queries.SetRuleLastRunParams{ID: r.ID, LastRun: pgtype.Timestamptz{Time: now, Valid: true}}); err != nil {
			return nil, err
		}
		ran[r.Name] = found
	}
	return ran, nil
}
