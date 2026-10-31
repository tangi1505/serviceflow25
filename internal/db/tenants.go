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
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db/queries"
)

// ErrNoDatabase says a tenant has no database configured (PL47d, Hauptplan E144).
var ErrNoDatabase = errors.New("db: tenant without a database")

// URLOf gives the URL of a tenant's database by the tenant's name ("nord" for root.nord), empty
// when none is configured: ITSM_TENANT_<NAME>_DATABASE_URL (platform.TenantDatabaseVar).
type URLOf func(name string) string

// tenantNames lists the tenants of the directory in the control database as path → name.
func tenantNames(ctx context.Context, control *pgxpool.Pool) (map[string]string, error) {
	rows, err := queries.New(control).ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: tenants: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Path] = r.Name
	}
	return out, nil
}

// urlFor is the URL of one tenant, or ErrNoDatabase naming the variable to set.
func urlFor(urlOf URLOf, name string) (string, error) {
	if u := urlOf(name); u != "" {
		return u, nil
	}
	return "", fmt.Errorf("%w: %s; set ITSM_TENANT_%s_DATABASE_URL", ErrNoDatabase, name, strings.ToUpper(name))
}

// MigrateTenants migrates the database of every tenant of the directory; one without its URL
// stops it.
func MigrateTenants(ctx context.Context, control *pgxpool.Pool, urlOf URLOf, logger *slog.Logger) error {
	names, err := tenantNames(ctx, control)
	if err != nil {
		return err
	}
	for path, name := range names {
		url, err := urlFor(urlOf, name)
		if err != nil {
			return err
		}
		logger.Info("migrating the database of a tenant", "tenant", path)
		if err := Migrate(ctx, url, logger); err != nil {
			return fmt.Errorf("tenant %s: %w", path, err)
		}
	}
	return nil
}

// OpenTenants opens the database of every tenant of the directory, by its path (PL47d). It
// is strict (your decision of 2026-10-30): a tenant without its URL stops the start, so that
// no tenant's data land in the control database by accident, and each database must stand at
// the migration of the control database. On an error nothing stays open.
func OpenTenants(ctx context.Context, control *pgxpool.Pool, urlOf URLOf) (map[string]*pgxpool.Pool, error) {
	names, err := tenantNames(ctx, control)
	if err != nil {
		return nil, err
	}
	want, err := Version(ctx, control)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*pgxpool.Pool, len(names))
	fail := func(err error) (map[string]*pgxpool.Pool, error) {
		for _, p := range out {
			p.Close()
		}
		return nil, err
	}
	for path, name := range names {
		url, err := urlFor(urlOf, name)
		if err != nil {
			return fail(err)
		}
		pool, err := Open(ctx, url)
		if err != nil {
			return fail(fmt.Errorf("tenant %s: %w", path, err))
		}
		out[path] = pool
		got, err := Version(ctx, pool)
		if err != nil {
			return fail(fmt.Errorf("tenant %s: %w", path, err))
		}
		if got != want {
			return fail(fmt.Errorf("db: the database of tenant %s stands at migration %d, the control database at %d; run itsm migrate", path, got, want))
		}
	}
	return out, nil
}

// Version is the newest migration applied to a database (0 before the first).
func Version(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var v int64
	err := pool.QueryRow(ctx, "SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied").Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("db: migration version: %w", err)
	}
	return v, nil
}

// BindFor binds the context to the database of a tenant given by its node (For), as
// Database.Bind does: for work of the system that writes into one tenant (seeding its lists).
func (r *Router) BindFor(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, boundKey{}, r.For(tenant))
}
