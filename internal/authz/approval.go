// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authz

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/query"
)

// --- approvals (WF6, WF-Plan E17) ---
//
// Whoever is asked to approve a record reads it, without a grant: the person, a member of an
// asked group, or a substitute while she stands in. A substitute stands in for a person from
// a time until a time (or for good), for the person and for her groups, and only at
// approvals: her other rights stay hers.

// Represented is a person the session's person stands in for, with her groups and the time.
type Represented struct {
	Person uuid.UUID   `json:"person"`
	Groups []uuid.UUID `json:"groups,omitempty"`
	From   time.Time   `json:"from"`
	Until  *time.Time  `json:"until,omitempty"`
}

// Active reports whether the substitution holds at now.
func (r Represented) Active(now time.Time) bool {
	return !now.Before(r.From) && (r.Until == nil || now.Before(*r.Until))
}

// Standing lists the persons the session's person stands in for at now.
func (a *Access) Standing(now time.Time) []Represented {
	if a == nil {
		return nil
	}
	var out []Represented
	for _, r := range a.Represents {
		if r.Active(now) {
			out = append(out, r)
		}
	}
	return out
}

// Asked lists the approvers the session decides for at now: the person, her groups, the
// persons she stands in for and their groups. Empty without a person.
func (a *Access) Asked(now time.Time) []uuid.UUID {
	if a == nil || a.PersonID == nil {
		return nil
	}
	out := a.involved()
	for _, r := range a.Standing(now) {
		out = append(out, r.Person)
		out = append(out, r.Groups...)
	}
	return out
}

// represents reads the substitutions of a person that have not ended, with the groups of the
// persons she stands in for.
func represents(ctx context.Context, home *pgxpool.Pool, groups *query.Groups, person uuid.UUID) ([]Represented, error) {
	rows, err := home.Query(ctx, `SELECT person_id, valid_from, valid_until FROM substitute
WHERE substitute_id = $1 AND (valid_until IS NULL OR valid_until > now()) ORDER BY valid_from`, person)
	if err != nil {
		return nil, fmt.Errorf("authz: substitutes: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Represented, error) {
		var r Represented
		err := row.Scan(&r.Person, &r.From, &r.Until)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("authz: substitutes: %w", err)
	}
	for i := range list {
		if list[i].Groups, err = groups.GroupsOfPerson(ctx, list[i].Person); err != nil {
			return nil, err
		}
	}
	return list, nil
}
