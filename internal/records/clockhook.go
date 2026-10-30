// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"

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

// clocksWritten tells the clocks of a write in its transaction.
func (s *Service) clocksWritten(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec Record) error {
	if s.Clocks == nil || !s.Clocks.Times(ctx, t.QualifiedName()) {
		return nil
	}
	return s.Clocks.Written(ctx, tx, t, id, rec)
}

// timed tells whether a type has clocks.
func (s *Service) timed(ctx context.Context, t meta.Type) bool {
	return s.Clocks != nil && s.Clocks.Times(ctx, t.QualifiedName())
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
