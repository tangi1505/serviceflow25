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

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- ratings (SD18, SD-Plan E12, N14) ---
//
// A record of a type with the trait rating can be rated by every person who reads it: helped
// or not, once, and changed at will. The record keeps its version; the sums are no secret of
// anyone, the vote of a person only hers.

// Rating is what a record got: the yes and no of all persons and the vote of the session's
// person, nil when she has none.
type Rating struct {
	Yes  int64 `json:"yes"`
	No   int64 `json:"no"`
	Mine *bool `json:"mine"`
}

func (s *Service) ratingType(ctx context.Context, typeName string) (meta.Type, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return meta.Type{}, err
	}
	if !t.HasTrait(meta.TraitRating) {
		return meta.Type{}, fmt.Errorf("%s has no rating (trait rating): %w", typeName, ErrNotFound)
	}
	return t, nil
}

// Rating returns the sums of a record the session may read.
func (s *Service) Rating(ctx context.Context, typeName string, id uuid.UUID) (Rating, error) {
	if _, err := s.ratingType(ctx, typeName); err != nil {
		return Rating{}, err
	}
	if _, err := s.Get(ctx, typeName, id); err != nil {
		return Rating{}, err
	}
	sums, err := s.Ratings(ctx, typeName, []uuid.UUID{id})
	return sums[id], err
}

// Rate sets the vote of the session's person on a record she may read; nil takes it back.
// Without a person there is no vote.
func (s *Service) Rate(ctx context.Context, typeName string, id uuid.UUID, helpful *bool) (Rating, error) {
	if _, err := s.ratingType(ctx, typeName); err != nil {
		return Rating{}, err
	}
	if _, err := s.Get(ctx, typeName, id); err != nil {
		return Rating{}, err
	}
	person := authz.From(ctx).PersonID
	if person == nil {
		return Rating{}, fmt.Errorf("%w: rating needs a person linked to the account", ErrForbidden)
	}
	q := queries.New(s.db.Tenant(ctx))
	var err error
	if helpful == nil {
		err = q.DeleteRating(ctx, queries.DeleteRatingParams{TypeName: typeName, RecordID: id, PersonID: *person})
	} else {
		err = q.UpsertRating(ctx, queries.UpsertRatingParams{TypeName: typeName, RecordID: id, PersonID: *person, Helpful: *helpful})
	}
	if err != nil {
		return Rating{}, fmt.Errorf("records: rate: %w", err)
	}
	sums, err := s.Ratings(ctx, typeName, []uuid.UUID{id})
	return sums[id], err
}

// Ratings returns the sums of records the caller has already read, with the vote of the
// session's person; a record without votes has zeros.
func (s *Service) Ratings(ctx context.Context, typeName string, ids []uuid.UUID) (map[uuid.UUID]Rating, error) {
	var person uuid.NullUUID
	if p := authz.From(ctx).PersonID; p != nil {
		person = uuid.NullUUID{UUID: *p, Valid: true}
	}
	rows, err := queries.New(s.db.Tenant(ctx)).RatingsOf(ctx, queries.RatingsOfParams{TypeName: typeName, RecordIds: ids, PersonID: person})
	if err != nil {
		return nil, fmt.Errorf("records: ratings: %w", err)
	}
	out := make(map[uuid.UUID]Rating, len(ids))
	for _, id := range ids {
		out[id] = Rating{}
	}
	for _, r := range rows {
		sum := Rating{Yes: r.Yes, No: r.No}
		if r.MineYes+r.MineNo > 0 {
			mine := r.MineYes > 0
			sum.Mine = &mine
		}
		out[r.RecordID] = sum
	}
	return out, nil
}
