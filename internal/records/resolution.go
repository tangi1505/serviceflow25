// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/meta"
)

// --- the trait resolution (UI14a, UI-Plan E40) ---
//
// A record of a type with the trait resolution keeps when it reached a state of the category
// done and how long it took from its creation: round the clock, and in working time by the
// calendar of its node without the time in states of the category waiting (taken from the
// history of the field state). Leaving the done states empties the three fields; the next
// resolution sets them anew. They are written in the transaction of the change of state.

// resolutionWritten sets or empties the fields of the trait resolution when a write moved the
// record into or out of the done states; rec then shows them.
func (s *Service) resolutionWritten(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, old, rec Record) error {
	if !t.HasTrait(meta.TraitResolution) || rec == nil {
		return nil
	}
	was, is := isDone(t, old[stateField]), isDone(t, rec[stateField])
	if was == is {
		return nil
	}
	sql := fmt.Sprintf("UPDATE %s SET %s = $2, %s = $3, %s = $4 WHERE id = $1", table(t),
		pgx.Identifier{meta.FieldResolvedAt}.Sanitize(),
		pgx.Identifier{meta.FieldResolutionHours}.Sanitize(),
		pgx.Identifier{meta.FieldResolutionServiceHours}.Sanitize())
	if !is {
		if _, err := tx.Exec(ctx, sql, id, nil, nil, nil); err != nil {
			return fmt.Errorf("records: resolution of %s: %w", t.QualifiedName(), err)
		}
		rec[meta.FieldResolvedAt], rec[meta.FieldResolutionHours], rec[meta.FieldResolutionServiceHours] = nil, nil, nil
		return nil
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return err
	}
	created, err := time.Parse(time.RFC3339Nano, fmt.Sprint(rec["created_at"]))
	if err != nil {
		return fmt.Errorf("records: resolution of %s: created_at: %w", t.QualifiedName(), err)
	}
	service, err := s.serviceTime(ctx, tx, t, id, rec, created, now)
	if err != nil {
		return err
	}
	hours := round2(now.Sub(created).Hours())
	serviceHours := round2(service.Hours())
	if _, err := tx.Exec(ctx, sql, id, now, hours, serviceHours); err != nil {
		return fmt.Errorf("records: resolution of %s: %w", t.QualifiedName(), err)
	}
	rec[meta.FieldResolvedAt] = now.UTC().Format(time.RFC3339Nano)
	rec[meta.FieldResolutionHours], rec[meta.FieldResolutionServiceHours] = hours, serviceHours
	return nil
}

// serviceTime is the working time from created to now without the stretches in a waiting
// state: the history of the field state gives the stretches, the calendar of the record's
// node counts them (round the clock without one).
func (s *Service) serviceTime(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec Record, created, now time.Time) (time.Duration, error) {
	rows, err := tx.Query(ctx, `SELECT at, old_value, new_value FROM audit
		WHERE type_name = $1 AND record_id = $2 AND field = 'state' ORDER BY at, version`, t.QualifiedName(), id)
	if err != nil {
		return 0, fmt.Errorf("records: history of %s: %w", t.QualifiedName(), err)
	}
	type move struct {
		at       time.Time
		from, to string
	}
	var moves []move
	for rows.Next() {
		var m move
		var from, to []byte
		if err := rows.Scan(&m.at, &from, &to); err != nil {
			rows.Close()
			return 0, err
		}
		_ = json.Unmarshal(from, &m.from)
		_ = json.Unmarshal(to, &m.to)
		moves = append(moves, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	path, _ := rec["org_path"].(string)
	count := func(a, b time.Time) (time.Duration, error) {
		if !b.After(a) {
			return 0, nil
		}
		if s.WorkingTime == nil {
			return b.Sub(a), nil
		}
		return s.WorkingTime(ctx, path, a, b)
	}
	// The stretches: from the creation in the first state to each move, the last up to now.
	state, from := initialState(t), created
	if len(moves) > 0 {
		state = moves[0].from
	}
	var total time.Duration
	for _, m := range append(moves, move{at: now}) {
		if !isWaiting(t, state) {
			d, err := count(from, m.at)
			if err != nil {
				return 0, err
			}
			total += d
		}
		if m.at.After(from) {
			from = m.at
		}
		state = m.to
	}
	return total, nil
}

func stateCategory(t meta.Type, v any) string {
	name, _ := v.(string)
	for _, st := range t.States {
		if st.Name == name {
			if st.Category == "" && st.Initial {
				return meta.CategoryOpen
			}
			return st.Category
		}
	}
	return ""
}

func isDone(t meta.Type, v any) bool { return stateCategory(t, v) == meta.CategoryDone }

func isWaiting(t meta.Type, v any) bool { return stateCategory(t, v) == meta.CategoryWaiting }

func initialState(t meta.Type) string {
	for _, st := range t.States {
		if st.Initial {
			return st.Name
		}
	}
	return ""
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
