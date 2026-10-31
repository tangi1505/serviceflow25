// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db/queries"
)

// ErrUsernameTaken says another account has the username, in whatever database.
var ErrUsernameTaken = errors.New("db: the username is taken")

// staleReservation is how old a reservation without its account must be before a new account
// may take the name over: the creation that made it broke off between the two databases.
const staleReservation = 10 * time.Minute

// Account is the database an account lives in and its tenant, "" for the installation
// (PL47b): with one database the control database; else the one account_home names. An
// account the directory does not know (made before PL47b, the system) is the installation's.
func (r *Router) Account(ctx context.Context, id uuid.UUID) (*pgxpool.Pool, string, error) {
	if !r.Separate() {
		return r.control, "", nil
	}
	h, err := queries.New(r.control).GetAccountHome(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.control, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("db: home of account %s: %w", id, err)
	}
	return r.For(h.Tenant), h.Tenant, nil
}

// User is the database of the account with the username, like Account; an unknown name gives
// the control database, where the lookup then finds nothing.
func (r *Router) User(ctx context.Context, username string) (*pgxpool.Pool, error) {
	if !r.Separate() {
		return r.control, nil
	}
	h, err := queries.New(r.control).GetAccountHomeByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.control, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: home of %s: %w", username, err)
	}
	return r.For(h.Tenant), nil
}

// Reserve claims the username for a new account in the tenant ("" for the installation) before
// the account is written to its database: usernames are unique across the installation
// (E138). A reservation older than ten minutes whose account never came is taken over; the
// caller releases its own when writing the account fails. Nothing to do with one database.
func (r *Router) Reserve(ctx context.Context, id uuid.UUID, username, tenant string) error {
	if !r.Separate() {
		return nil
	}
	q := queries.New(r.control)
	t := pgtype.Text{String: tenant, Valid: tenant != ""}
	err := q.ReserveAccountHome(ctx, queries.ReserveAccountHomeParams{AccountID: id, Username: username, Tenant: t})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		if err != nil {
			return fmt.Errorf("db: reserve %s: %w", username, err)
		}
		return nil
	}
	h, err := q.GetAccountHomeByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("db: reservation of %s: %w", username, err)
	}
	if time.Since(h.CreatedAt.Time) < staleReservation {
		return ErrUsernameTaken
	}
	if _, err := queries.New(r.For(h.Tenant)).GetAccount(ctx, h.AccountID); !errors.Is(err, pgx.ErrNoRows) {
		return ErrUsernameTaken
	}
	if err := q.DeleteAccountHome(ctx, h.AccountID); err != nil {
		return fmt.Errorf("db: stale reservation of %s: %w", username, err)
	}
	if err := q.ReserveAccountHome(ctx, queries.ReserveAccountHomeParams{AccountID: id, Username: username, Tenant: t}); err != nil {
		return fmt.Errorf("db: reserve %s: %w", username, err)
	}
	return nil
}

// Release gives a reservation back, after the account could not be written.
func (r *Router) Release(ctx context.Context, id uuid.UUID) {
	if r.Separate() {
		_ = queries.New(r.control).DeleteAccountHome(context.WithoutCancel(ctx), id)
	}
}

// Taken reports whether an account has the username anywhere in the installation.
func (r *Router) Taken(ctx context.Context, username string) (bool, error) {
	if !r.Separate() {
		return queries.New(r.control).UsernameTaken(ctx, username)
	}
	_, err := queries.New(r.control).GetAccountHomeByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
