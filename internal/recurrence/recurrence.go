// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package recurrence creates recurring records (WF14; Inventar G61, G62). A recurrence is a
// record of the core type core.recurrence: a target type, a cron schedule in a time zone and a
// template. A job every minute takes the recurrences due (next_run reached, or not yet set),
// creates one record per date and moves next_run to the next date after now, so that dates
// missed while no instance ran are made up once. The record carries the source
// "recurrence:<id>" and the date as its key; the unique key of the bulk (PL31) makes it
// exactly once, also with two instances at the same minute.
package recurrence

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// TypeRecurrence is the core type of the recurrences.
const TypeRecurrence = "core.recurrence"

// Service runs the recurrences.
type Service struct {
	rec    *records.Service
	logger *slog.Logger
}

// New wires the service.
func New(rec *records.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{rec: rec, logger: logger}
}

// RunArgs is the job every minute.
type RunArgs struct{}

// Kind names the job.
func (RunArgs) Kind() string { return "recurrence.run" }

// Register adds the job every minute and its worker.
func (s *Service) Register(js *jobs.Service) error {
	jobs.AddWorker(js, &runWorker{s: s})
	return js.AddPeriodic(jobs.Periodic{
		Name:     RunArgs{}.Kind(),
		Label:    map[string]string{"de": "Wiederkehrende Vorgänge", "en": "Recurring records"},
		Schedule: "* * * * *",
		Queue:    jobs.QueueDefault,
		Args:     func() river.JobArgs { return RunArgs{} },
	})
}

type runWorker struct {
	river.WorkerDefaults[RunArgs]
	s *Service
}

// Work runs the recurrences due now.
func (w *runWorker) Work(ctx context.Context, _ *river.Job[RunArgs]) error {
	created, err := w.s.Run(ctx, time.Now())
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"created": created})
}

// schedule parses a cron schedule in a time zone.
func schedule(expr, tz string) (cron.Schedule, *time.Location, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		return nil, nil, fmt.Errorf("time zone %q is unknown", tz)
	}
	sched, err := cron.ParseStandard(strings.TrimSpace(expr))
	if err != nil || strings.Contains(expr, "TZ=") {
		return nil, nil, fmt.Errorf("schedule %q is no cron expression like \"0 7 * * 1\"", expr)
	}
	return sched, loc, nil
}

// Check is the RecordCheck of records for core.recurrence: the target type is installed, the
// schedule and the time zone parse, and the template, filled in for a date, is a valid create
// of the target type.
func (s *Service) Check(ctx context.Context, t meta.Type, rec map[string]any) []records.Problem {
	if t.QualifiedName() != TypeRecurrence {
		return nil
	}
	var ps []records.Problem
	add := func(field, code, msg string) {
		ps = append(ps, records.Problem{Field: field, Code: code, Message: msg})
	}
	tz := fmt.Sprint(rec["timezone"])
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		add("timezone", "type", fmt.Sprintf("time zone %q is unknown", tz))
		loc = time.UTC
	} else if _, _, err := schedule(fmt.Sprint(rec["schedule"]), tz); err != nil {
		add("schedule", "type", err.Error())
	}
	name, _ := rec["target_type"].(string)
	target, err := s.rec.Type(ctx, name)
	if err != nil {
		add("target_type", "unknown", fmt.Sprintf("type %s is not installed", name))
		return ps
	}
	values, err := fill(fmt.Sprint(rec["template"]), time.Date(2026, 1, 5, 7, 0, 0, 0, loc), target)
	if err != nil {
		add("template", "type", err.Error())
		return ps
	}
	if err := s.rec.CheckValues(authz.WithAccess(ctx, authz.System()), name, values, true); err != nil {
		add("template", "invalid", err.Error())
	}
	return ps
}

// Run creates the records of the recurrences due at now and returns how many it created.
func (s *Service) Run(ctx context.Context, now time.Time) (int, error) {
	ctx = authz.WithAccess(ctx, authz.System())
	where, _ := json.Marshal(map[string]any{"and": []any{
		map[string]any{"field": "active", "eq": true},
		map[string]any{"or": []any{
			map[string]any{"field": "next_run", "null": true},
			map[string]any{"field": "next_run", "lte": now.UTC().Format(time.RFC3339Nano)},
		}},
	}})
	created := 0
	after := ""
	for {
		page, _, err := s.rec.Query(ctx, TypeRecurrence, records.Query{Where: where, Limit: 500, After: after})
		if err != nil {
			return created, fmt.Errorf("recurrence: due: %w", err)
		}
		for _, r := range page.Items {
			made, err := s.one(ctx, r, now)
			if err != nil {
				return created, err
			}
			if made {
				created++
			}
		}
		if page.Next == "" {
			return created, nil
		}
		after = page.Next
	}
}

// one runs one recurrence due: it creates the record of the date in next_run, if any, and
// moves next_run on. A template the target type refuses leaves its error in last_error; the
// recurrence goes on with the next date.
func (s *Service) one(ctx context.Context, r records.Record, now time.Time) (bool, error) {
	id, err := uuid.Parse(fmt.Sprint(r["id"]))
	if err != nil {
		return false, err
	}
	patch := map[string]any{}
	sched, loc, err := schedule(fmt.Sprint(r["schedule"]), fmt.Sprint(r["timezone"]))
	if err != nil {
		patch["last_error"], patch["active"] = err.Error(), false
		_, err = s.rec.Update(ctx, TypeRecurrence, id, -1, patch, nil)
		return false, err
	}
	made := false
	if date, ok := timeOf(r["next_run"]); ok {
		target := fmt.Sprint(r["target_type"])
		var problem string
		t, err := s.rec.Type(ctx, target)
		if err != nil {
			problem = err.Error()
		} else if values, err := fill(fmt.Sprint(r["template"]), date.In(loc), t); err != nil {
			problem = err.Error()
		} else {
			raw, _ := json.Marshal(values)
			sum := sha256.Sum256(raw)
			key := records.SyncKey{Source: "recurrence:" + id.String(), Key: date.UTC().Format(time.RFC3339), Hash: sum[:]}
			_, _, status, err := s.rec.Sync(ctx, target, key, values, nil)
			switch {
			case errors.Is(err, records.ErrConflict):
				// Another instance created it at the same minute.
			case err != nil:
				if _, ok := records.AsValidationError(err); !ok {
					return false, err
				}
				problem = err.Error()
			default:
				made = status == records.SyncCreated
			}
		}
		patch["last_run"] = date.UTC().Format(time.RFC3339)
		if problem != "" {
			s.logger.Warn("recurrence: not created", "recurrence", r["name"], "date", date, "error", problem)
			patch["last_error"] = problem
		} else if r["last_error"] != nil {
			patch["last_error"] = nil
		}
	}
	// The next date after now: missed dates are made up once, not one by one.
	patch["next_run"] = sched.Next(now.In(loc)).UTC().Format(time.RFC3339)
	_, err = s.rec.Update(ctx, TypeRecurrence, id, -1, patch, nil)
	return made, err
}

// offsetRe is a date relative to the date of the recurrence: +3d, +4h, +1w.
var offsetRe = regexp.MustCompile(`^\+(\d{1,4})([hdw])$`)

// fill turns the template into the values of a create for one date: placeholders in text,
// offsets in date and datetime fields.
func fill(template string, date time.Time, t meta.Type) (map[string]any, error) {
	var values map[string]any
	if err := json.Unmarshal([]byte(template), &values); err != nil || values == nil {
		return nil, errors.New("the template must be a JSON object, as in a create")
	}
	_, week := date.ISOWeek()
	repl := strings.NewReplacer("{{date}}", date.Format(time.DateOnly), "{{week}}", strconv.Itoa(week),
		"{{month}}", fmt.Sprintf("%02d", int(date.Month())), "{{year}}", strconv.Itoa(date.Year()))
	for k, v := range values {
		str, ok := v.(string)
		if !ok {
			continue
		}
		if f, ok := t.Field(k); ok && (f.Type == meta.FieldDate || f.Type == meta.FieldDateTime) {
			if m := offsetRe.FindStringSubmatch(str); m != nil {
				n, _ := strconv.Atoi(m[1])
				var at time.Time
				switch m[2] {
				case "h":
					at = date.Add(time.Duration(n) * time.Hour)
				case "d":
					at = date.AddDate(0, 0, n)
				default:
					at = date.AddDate(0, 0, 7*n)
				}
				if f.Type == meta.FieldDate {
					values[k] = at.Format(time.DateOnly)
				} else {
					values[k] = at.UTC().Format(time.RFC3339)
				}
				continue
			}
		}
		values[k] = repl.Replace(str)
	}
	return values, nil
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
