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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
)

// --- the hooks of the SLA clocks (WF9; architektur.md §8) ---
//
// The clocks start, pause and stop in the transaction of the write that makes them do so, so
// that a pause begins with the change of state and the remaining time is right as soon as the
// write returns. Bulk rows of a type with clocks go the way of Sync for the same reason.

// Clocks is the hook of the SLA clocks.
type Clocks interface {
	// Times tells whether records of a type have clocks.
	Times(ctx context.Context, typeName string) bool
	// Written runs in the transaction of every write of such a record, with the record as it
	// is after the write (in the trash with deleted_at; nil after a purge).
	Written(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec Record) error
}

// RecordCheck checks a record as it will be written beyond its fields, such as the conditions
// of an SLA; problems refuse the write with 422. Types it does not know pass.
type RecordCheck func(ctx context.Context, t meta.Type, rec map[string]any) []Problem

// Checks runs several RecordChecks, each for the types it knows (WF9 SLAs, WF14 recurrences).
func Checks(list ...RecordCheck) RecordCheck {
	return func(ctx context.Context, t meta.Type, rec map[string]any) []Problem {
		var ps []Problem
		for _, c := range list {
			ps = append(ps, c(ctx, t, rec)...)
		}
		return ps
	}
}

// clocksWritten tells the clocks of a write in its transaction; the record then shows the
// deadline the clocks left (UI34).
func (s *Service) clocksWritten(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec Record) error {
	if s.Clocks == nil || !s.Clocks.Times(ctx, t.QualifiedName()) {
		return nil
	}
	if err := s.Clocks.Written(ctx, tx, t, id, rec); err != nil || rec == nil {
		return err
	}
	var due, warn *time.Time
	var paused bool
	err := tx.QueryRow(ctx, fmt.Sprintf("SELECT sla_due_at, sla_warn_at, sla_paused FROM %s WHERE id = $1", table(t)), id).
		Scan(&due, &warn, &paused)
	if err != nil {
		return fmt.Errorf("records: deadline of %s: %w", t.QualifiedName(), err)
	}
	moment := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return systemToJSON("", *t)
	}
	rec["sla_due_at"], rec["sla_warn_at"], rec["sla_paused"] = moment(due), moment(warn), paused
	return nil
}

// Deadline is what the clocks of a record leave for its list (UI34, UI-Plan E37): the next
// deadline of a running clock and the moment it has used three quarters of its target; Paused
// when no clock runs but one waits.
type Deadline struct {
	Due, Warn *time.Time
	Paused    bool
}

// SetDeadlineTx writes the deadline of a record in the transaction of the clocks. It is no
// change of the record: no version, no history, no event.
func (s *Service) SetDeadlineTx(ctx context.Context, tx pgx.Tx, typeName string, id uuid.UUID, d Deadline) error {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET sla_due_at = $2, sla_warn_at = $3, sla_paused = $4
WHERE id = $1 AND (sla_due_at IS DISTINCT FROM $2 OR sla_warn_at IS DISTINCT FROM $3 OR sla_paused <> $4)`, table(t)),
		id, d.Due, d.Warn, d.Paused)
	if err != nil {
		return fmt.Errorf("records: set deadline of %s: %w", typeName, err)
	}
	return nil
}

// timed tells whether a type has clocks.
func (s *Service) timed(ctx context.Context, t meta.Type) bool {
	return s.Clocks != nil && s.Clocks.Times(ctx, t.QualifiedName())
}

// Timed is the type as the API shows it, marked when its records have clocks (UI34).
func (s *Service) Timed(ctx context.Context, t meta.Type) meta.Type {
	t.Timed = s.timed(ctx, t)
	return t
}

// check runs the RecordCheck on a record as it will be.
func (s *Service) check(ctx context.Context, t meta.Type, rec map[string]any) error {
	if s.CheckRecord == nil {
		return nil
	}
	if ps := s.CheckRecord(ctx, t, rec); len(ps) > 0 {
		return &ValidationError{Problems: ps}
	}
	return nil
}

// TraceTx is Trace inside a transaction of the caller, with the rights of the system: the
// clocks note a breach with it in the transaction that marks the clock (WF9). The caller
// reports the write with Touched after the commit.
func (s *Service) TraceTx(ctx context.Context, tx pgx.Tx, typeName string, id uuid.UUID, action string, payload map[string]any) error {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return err
	}
	return trace(authz.WithAccess(ctx, authz.System()), tx, s, t, id, action, payload, nil)
}

// Touched reports a write the caller made in a transaction of its own (TraceTx).
func (s *Service) Touched(ctx context.Context, typeName string) {
	s.afterWrite(ctx, typeName)
}
