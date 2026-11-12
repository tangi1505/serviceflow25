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
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/rules"
)

// --- changes that wait for approval (SD17, SD-Plan N22) ---
//
// A type may hold changes of some fields back (approval.hold): while the condition holds for
// the record (a published article), a write that changes one of these fields writes the rest at
// once and keeps the change of these fields at a new approval, asked of the holders of a role
// of the record in one stage where one yes is enough. Readers keep seeing the record as it was.
// Given, the change is written as a new version with the rights of the system and the one who
// asked as actor; refused or withdrawn, it is dropped. While it waits, the held fields cannot be
// changed again (409): one change at a time, decided as it was asked (your decision 2A).

// heldKey marks the write of a held change once it is given: it is not held again.
type heldKey struct{}

// holdBack takes the changes of held fields out of values and tr and asks for their approval,
// when the type holds them and its condition holds for the record. It reports whether it did.
func (s *Service) holdBack(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, old Record, raw, values map[string]any, tr translations, actor Actor) (bool, error) {
	h := approvalSpec(t).Hold
	if h == nil || ctx.Value(heldKey{}) != nil {
		return false, nil
	}
	if h.When != "" {
		cond, err := rules.Compile(h.When)
		if err != nil {
			return false, fmt.Errorf("records: hold of %s: %w", t.QualifiedName(), err)
		}
		holds, err := cond.Eval(map[string]any(old))
		if err != nil {
			return false, fmt.Errorf("records: hold of %s: %w", t.QualifiedName(), err)
		}
		if !holds {
			return false, nil
		}
	}
	held := map[string]any{}
	for _, name := range h.Fields {
		f, ok := t.Field(name)
		if !ok {
			continue
		}
		v, set := values[name]
		changed := set && !equalJSON(old[name], toJSON(f, v))
		if _, other := tr[name]; changed || other {
			if r, given := raw[name]; given {
				held[name] = r
			} else {
				held[name] = v
			}
			delete(values, name)
			delete(tr, name)
		}
	}
	if len(held) == 0 {
		return false, nil
	}
	var open int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM approval WHERE type_name = $1 AND record_id = $2 AND state = 'open'`,
		t.QualifiedName(), id).Scan(&open); err != nil {
		return false, fmt.Errorf("records: hold: %w", err)
	}
	if open > 0 {
		return false, fmt.Errorf("%w: a change of %s waits for approval; withdraw it first", ErrConflict, strings.Join(slices.Sorted(maps.Keys(held)), ", "))
	}
	all, _ := old[actorsField].(map[string]any)
	holders, _ := all[h.Approvers].([]any)
	var approvers []Ref
	for _, x := range holders {
		m, _ := x.(map[string]any)
		hid, err := uuid.Parse(fmt.Sprint(m["id"]))
		if err != nil {
			continue
		}
		approvers = append(approvers, Ref{Type: fmt.Sprint(m["type"]), ID: hid})
	}
	if len(approvers) == 0 {
		return false, &ValidationError{Problems: []Problem{{Field: actorsField + "." + h.Approvers, Code: "required",
			Message: "nobody holds the role that approves a change of " + strings.Join(slices.Sorted(maps.Keys(held)), ", ")}}}
	}
	changes, err := json.Marshal(held)
	if err != nil {
		return false, err
	}
	req := ApprovalRequest{Message: h.Message, Stages: []StageRequest{{Need: 1, Approvers: approvers}}}
	aid, err := s.insertApproval(ctx, tx, t, id, old, req, changes, actor)
	if err != nil {
		return false, err
	}
	return true, s.approvalWritten(ctx, tx, t, id, old, meta.ApprovalPending, EventApprovalRequest,
		map[string]any{"approval": aid.String(), "requested": true, "changes": slices.Sorted(maps.Keys(held))},
		map[string]any{"approval": aid.String(), "stage": 1, "approvers": approvers}, actor)
}

// applyHeld writes a given change as a new version, with the rights of the system and the one
// who asked as actor. When it cannot (a field changed its rules meanwhile), the history says so
// and the approval stands.
func (s *Service) applyHeld(ctx context.Context, t meta.Type, id uuid.UUID, changes map[string]any, actor Actor) {
	system := context.WithValue(authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant())), heldKey{}, true)
	if _, err := s.update(system, t, id, -1, changes, actor, nil, nil); err != nil {
		slog.WarnContext(ctx, "approval: the change was not written", "type", t.QualifiedName(), "id", id, "error", err)
		_ = s.Trace(system, t.QualifiedName(), id, EventApproval, map[string]any{"changes": "not written", "failed": err.Error()}, actor)
	}
}
