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
	"reflect"

	"github.com/google/uuid"
)

// fixedIDKey carries the id a mirror is created under.
type fixedIDKey struct{}

// Mirror creates the record under the given id in the database of the request's tenant, or
// brings the given fields of the one there up to date (PL47b, Hauptplan E141): the person of an
// administrator of the installation is mirrored into every tenant with a database of its own,
// so that the tenant's records point at a person the database knows. A trashed mirror is left
// alone. It returns whether something was written.
func (s *Service) Mirror(ctx context.Context, typeName string, id uuid.UUID, values map[string]any, actor Actor) (bool, error) {
	rec, err := s.Get(ctx, typeName, id)
	if errors.Is(err, ErrNotFound) {
		t, err := s.Type(ctx, typeName)
		if err != nil {
			return false, err
		}
		if _, err := s.read(ctx, s.db.Tenant(ctx), t, id, true, false); err == nil {
			return false, nil
		}
		if _, err := s.create(context.WithValue(ctx, fixedIDKey{}, id), t, values, actor, nil, nil); err != nil {
			return false, fmt.Errorf("records: mirror %s %s: %w", typeName, id, err)
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	patch := map[string]any{}
	for k, v := range values {
		if k == orgPathField {
			continue
		}
		if !reflect.DeepEqual(rec[k], v) {
			patch[k] = v
		}
	}
	if len(patch) == 0 {
		return false, nil
	}
	version, _ := rec["version"].(int64)
	if _, err := s.Update(ctx, typeName, id, version, patch, actor); err != nil {
		return false, fmt.Errorf("records: mirror %s %s: %w", typeName, id, err)
	}
	return true, nil
}
