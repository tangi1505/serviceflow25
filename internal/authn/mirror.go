// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authn

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/records"
)

// Mirrors reports whether administrators of the installation get mirrors in the tenants: only
// with a database per tenant (PL47b).
func (s *Service) Mirrors() bool { return s.db.Separate() }

// Mirror brings the mirror of an administrator of the installation up to date in a tenant
// with a database of its own (PL47b, Hauptplan E141): account and person under the same ids as
// the originals, which stay central, the account without a password. The tenant's records may
// then name the administrator as an actor or a creator, and the database checks the reference.
// It does nothing with one database, for an account of a tenant and for the control database.
func (s *Service) Mirror(ctx context.Context, accountID uuid.UUID, tenant string) error {
	if !s.db.Separate() || tenant == "" {
		return nil
	}
	target := s.db.For(tenant)
	if target == s.db.Control() {
		return nil
	}
	home, _, err := s.db.Account(ctx, accountID)
	if err != nil {
		return err
	}
	if home != s.db.Control() {
		return nil
	}
	// The browser sends several requests at once right after a switch, and each brings the
	// mirror up to date. Two that create it at the same moment collide on a unique key; the
	// one that loses tries once more and then finds the mirror there (Nachtrag 2026-11-03).
	err = s.mirror(ctx, home, target, accountID, tenant)
	var pgErr *pgconn.PgError
	if errors.Is(err, records.ErrConflict) || errors.As(err, &pgErr) && pgErr.Code == "23505" {
		err = s.mirror(ctx, home, target, accountID, tenant)
	}
	return err
}

// mirror writes the mirror of an account and its person in a tenant's database once.
func (s *Service) mirror(ctx context.Context, home, target *pgxpool.Pool, accountID uuid.UUID, tenant string) error {
	acc, err := queries.New(home).GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("authn: account to mirror: %w", err)
	}
	if acc.PersonID.Valid {
		central, err := s.rec.Get(authz.WithAccess(ctx, authz.System()), "core.person", acc.PersonID.UUID)
		if err != nil {
			return fmt.Errorf("authn: person to mirror: %w", err)
		}
		values := map[string]any{"org_path": tenant}
		for _, k := range []string{"last_name", "first_name", "email"} {
			if v, ok := central[k]; ok && v != nil && v != "" {
				values[k] = v
			}
		}
		if _, err := s.rec.Mirror(authz.WithAccess(ctx, authz.SystemIn(tenant)), "core.person", acc.PersonID.UUID, values, nil); err != nil {
			return err
		}
	}
	if err := queries.New(target).UpsertMirrorAccount(ctx, queries.UpsertMirrorAccountParams{
		ID: acc.ID, Username: acc.Username, DisplayName: acc.DisplayName, DisabledAt: acc.DisabledAt, PersonID: acc.PersonID,
	}); err != nil {
		return fmt.Errorf("authn: mirror account: %w", err)
	}
	return nil
}
