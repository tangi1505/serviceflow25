// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- rollups: sums and counts over the records that point here (WF13) ---
//
// A write of a record whose type others count (a task) recomputes the rollup fields of the
// records it points to, before and after the write (a task moved to another record changes
// both), in the same transaction. The row of each such record is locked first, in the order
// of the ids, and counted in a statement of its own after the lock, so that two tasks of one
// record written at once each count the other. The record keeps its version, as with the sums
// of the costs; the history shows the new value, and an event "update" names the fields, so
// that rules and clocks of the record see it.

// rollupTarget is a rollup field and the type it belongs to.
type rollupTarget struct {
	parent meta.Type
	field  meta.Field
}

// rollupsOf returns the rollup fields that count records of t (Type.CountedBy).
func (s *Service) rollupsOf(ctx context.Context, t meta.Type) ([]rollupTarget, error) {
	var out []rollupTarget
	for _, ref := range t.CountedBy {
		dot := strings.LastIndex(ref, ".")
		p, err := s.Type(ctx, ref[:dot])
		if err != nil {
			return nil, err
		}
		f, ok := p.Field(ref[dot+1:])
		if !ok || f.Rollup == nil {
			return nil, fmt.Errorf("records: rollup %s is not installed", ref)
		}
		out = append(out, rollupTarget{parent: p, field: f})
	}
	return out, nil
}

// rolledUp tells whether other types count records of t.
func (s *Service) rolledUp(_ context.Context, t meta.Type) bool {
	return len(t.CountedBy) > 0
}

// rollupsWritten recomputes the rollups at the records old and rec point to (old nil on a
// create, rec in the trash with deleted_at).
func (s *Service) rollupsWritten(ctx context.Context, tx pgx.Tx, t meta.Type, old, rec Record, actor Actor) error {
	targets, err := s.rollupsOf(ctx, t)
	if err != nil || len(targets) == 0 {
		return err
	}
	type key struct {
		typ string
		id  uuid.UUID
	}
	fields := map[key][]meta.Field{}
	types := map[string]meta.Type{}
	for _, tg := range targets {
		for _, r := range []Record{old, rec} {
			if r == nil {
				continue
			}
			id, err := uuid.Parse(fmt.Sprint(r[tg.field.Rollup.Via]))
			if err != nil {
				continue // no record pointed to
			}
			k := key{tg.parent.QualifiedName(), id}
			if !hasField(fields[k], tg.field.Name) {
				fields[k] = append(fields[k], tg.field)
			}
			types[k.typ] = tg.parent
		}
	}
	keys := make([]key, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		return keys[i].id.String() < keys[j].id.String()
	})
	q := queries.New(tx)
	for _, k := range keys {
		pt := types[k.typ]
		prec, err := s.read(ctx, tx, pt, k.id, true, true)
		if errors.Is(err, ErrNotFound) {
			continue // purged meanwhile
		}
		if err != nil {
			return err
		}
		var changes []change
		var sets []string
		args := []any{k.id}
		for _, f := range fields[k] {
			v, err := rollupValue(ctx, tx, t, f.Rollup, k.id)
			if err != nil {
				return fmt.Errorf("records: rollup %s.%s: %w", k.typ, f.Name, err)
			}
			next := toJSON(f, v)
			if equalJSON(prec[f.Name], next) {
				continue
			}
			args = append(args, v)
			sets = append(sets, fmt.Sprintf("%s = $%d", pgx.Identifier{f.Name}.Sanitize(), len(args)))
			changes = append(changes, change{f.Name, prec[f.Name], next})
		}
		if len(changes) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s WHERE id = $1", table(pt), strings.Join(sets, ", ")), args...); err != nil {
			return fmt.Errorf("records: rollup %s: %w", k.typ, err)
		}
		version := prec["version"].(int64)
		changed := make([]string, 0, len(changes))
		for i := range changes {
			if err := audit(ctx, q, pt, k.id, version, "update", &changes[i], actor); err != nil {
				return err
			}
			changed = append(changed, changes[i].field)
		}
		after, err := s.read(ctx, tx, pt, k.id, true, false)
		if err != nil {
			return err
		}
		if err := event(ctx, q, pt, k.id, after, "update", version, changed, nil, actor); err != nil {
			return err
		}
		// A clock of the record may stop on its sums, such as when no task is open (WF9).
		if err := s.clocksWritten(ctx, tx, pt, k.id, after); err != nil {
			return err
		}
	}
	return nil
}

// rollupValue counts or sums the records of from that point to parent, after the lock on
// the parent's row (a statement of its own sees what committed before).
func rollupValue(ctx context.Context, tx pgx.Tx, from meta.Type, r *meta.Rollup, parent uuid.UUID) (float64, error) {
	agg := "count(*)"
	if r.Op == meta.RollupSum {
		agg = fmt.Sprintf("coalesce(sum(%s), 0)", pgx.Identifier{r.Field}.Sanitize())
	}
	where, args, err := authz.Translate(r.Where, []any{parent})
	if err != nil {
		return 0, err
	}
	sql := fmt.Sprintf("SELECT (%s)::float8 FROM %s WHERE %s = $1 AND deleted_at IS NULL AND (%s)",
		agg, table(from), pgx.Identifier{r.Via}.Sanitize(), where)
	var v float64
	if err := tx.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func hasField(list []meta.Field, name string) bool {
	for _, f := range list {
		if f.Name == name {
			return true
		}
	}
	return false
}
