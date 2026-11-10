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
)

// --- substitutes at approvals (WF6, WF-Plan E17) ---
//
// A person names who stands in for her at approvals, from a time until a time or for good;
// the administration does it at the person (the right to change the person record). While it
// lasts the substitute decides for her and her groups (authz.Access.Asked). Rights change
// with it: the caller invalidates the cached access after a change.

// PersonType is the core type of persons.
const PersonType = "core.person"

// Substitute is one substitution of a person.
type Substitute struct {
	ID         uuid.UUID  `json:"id"`
	Person     uuid.UUID  `json:"person"`
	Substitute uuid.UUID  `json:"substitute"`
	From       time.Time  `json:"valid_from"`
	Until      *time.Time `json:"valid_until,omitempty"`
	Active     bool       `json:"active"`
}

// checkSubstitutes allows the person herself, or whoever may change her record.
func (s *Service) checkSubstitutes(ctx context.Context, person uuid.UUID) error {
	a := authz.From(ctx)
	if me := a.Me(); me != nil && *me == person {
		return nil
	}
	_, err := s.Check(ctx, PersonType, person, authz.ActionUpdate)
	return err
}

// Substitutes lists the substitutions of a person, those not over first.
func (s *Service) Substitutes(ctx context.Context, person uuid.UUID) ([]Substitute, error) {
	if err := s.checkSubstitutes(ctx, person); err != nil {
		return nil, err
	}
	rows, err := s.db.Tenant(ctx).Query(ctx, `SELECT id, person_id, substitute_id, valid_from, valid_until FROM substitute
WHERE person_id = $1 ORDER BY (valid_until IS NOT NULL AND valid_until <= now()), valid_from, id`, person)
	if err != nil {
		return nil, fmt.Errorf("records: substitutes: %w", err)
	}
	now := time.Now()
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Substitute, error) {
		var x Substitute
		err := row.Scan(&x.ID, &x.Person, &x.Substitute, &x.From, &x.Until)
		x.Active = !now.Before(x.From) && (x.Until == nil || now.Before(*x.Until))
		return x, err
	})
	if err != nil {
		return nil, fmt.Errorf("records: substitutes: %w", err)
	}
	if list == nil {
		list = []Substitute{}
	}
	return list, nil
}

// AddSubstitute names a substitute of a person; from defaults to now, until may be empty.
func (s *Service) AddSubstitute(ctx context.Context, person, substitute uuid.UUID, from, until *time.Time, actor Actor) (Substitute, error) {
	if err := s.checkSubstitutes(ctx, person); err != nil {
		return Substitute{}, err
	}
	var ps []Problem
	if substitute == person {
		ps = append(ps, Problem{Field: "substitute", Code: "invalid", Message: "a person cannot stand in for herself"})
	} else {
		system := authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant()))
		ok, err := s.exists(system, PersonType, substitute)
		if err != nil {
			return Substitute{}, err
		}
		if !ok {
			ps = append(ps, Problem{Field: "substitute", Code: "not_found", Message: fmt.Sprintf("person %s does not exist", substitute)})
		}
	}
	start := time.Now()
	if from != nil {
		start = *from
	}
	if until != nil && !until.After(start) {
		ps = append(ps, Problem{Field: "valid_until", Code: "invalid", Message: "must be after valid_from"})
	}
	if len(ps) > 0 {
		return Substitute{}, &ValidationError{Problems: ps}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Substitute{}, err
	}
	if _, err := s.db.Tenant(ctx).Exec(ctx, `INSERT INTO substitute (id, person_id, substitute_id, valid_from, valid_until, created_by)
VALUES ($1, $2, $3, $4, $5, $6)`, id, person, substitute, start, until, nullActor(actor)); err != nil {
		return Substitute{}, fmt.Errorf("records: add substitute: %w", err)
	}
	now := time.Now()
	return Substitute{ID: id, Person: person, Substitute: substitute, From: start, Until: until,
		Active: !now.Before(start) && (until == nil || now.Before(*until))}, nil
}

// RemoveSubstitute ends a substitution of a person by deleting it.
func (s *Service) RemoveSubstitute(ctx context.Context, person, id uuid.UUID) error {
	if err := s.checkSubstitutes(ctx, person); err != nil {
		return err
	}
	tag, err := s.db.Tenant(ctx).Exec(ctx, `DELETE FROM substitute WHERE id = $1 AND person_id = $2`, id, person)
	if err != nil {
		return fmt.Errorf("records: remove substitute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("substitute %s: %w", id, ErrNotFound)
	}
	return nil
}
