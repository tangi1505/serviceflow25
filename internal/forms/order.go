// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package forms

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/relations"
)

// --- ordering an offer (SD13, SD-Plan N19) ---
//
// A type with the block order (meta.OrderSpec) is orderable: an offer of the catalog names the
// form a person fills in. Ordering reads the offer with the rights of the person (who does not
// see it cannot order it), submits its form like any form, with the values the offer hands on
// (its template, delivery time and costs) before the answers, and links the new record to the
// offer with the kind the type names. The form may be offered only through offers (place
// "order"); it must be active, and the person must be allowed to create its target.

// Order creates the record of an offer's form from the answers.
func (s *Service) Order(ctx context.Context, typeName string, id uuid.UUID, answers map[string]any, actor records.Actor) (records.Record, error) {
	t, err := s.rec.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	spec := t.Order
	if spec == nil {
		return nil, fmt.Errorf("%s cannot be ordered: %w", typeName, ErrNotFound)
	}
	offer, err := s.rec.Get(ctx, typeName, id)
	if err != nil {
		return nil, err
	}
	formID, err := uuid.Parse(fmt.Sprint(offer[spec.Form]))
	if err != nil {
		return nil, &records.ValidationError{Problems: []records.Problem{{Field: spec.Form, Code: "required", Message: "the offer names no form to order with"}}}
	}
	f, err := s.Get(ctx, formID)
	if err != nil {
		return nil, err
	}
	target, err := s.rec.Type(ctx, f.TargetType)
	if err != nil {
		return nil, err
	}
	given := map[string]any{}
	for to, from := range spec.Copy {
		if _, ok := target.Field(to); !ok {
			continue // a form for another type: what it lacks is not handed on
		}
		if v := offer[from]; v != nil && v != "" {
			given[to] = v
		}
	}
	rec, err := s.submit(ctx, f, answers, given, actor)
	if err != nil || spec.Relation == "" || s.rel == nil {
		return rec, err
	}
	newID, err := uuid.Parse(fmt.Sprint(rec["id"]))
	if err != nil {
		return rec, err
	}
	// The link is the order's, not a change the person makes at the offer: system rights.
	from := relations.End{Type: f.TargetType, ID: newID}
	to := relations.End{Type: typeName, ID: id}
	if _, err := s.rel.Create(authz.WithAccess(ctx, authz.System()), spec.Relation, from, to, actor); err != nil {
		return rec, fmt.Errorf("order: %s exists, but linking it to the offer failed: %w", from, err)
	}
	return rec, nil
}
