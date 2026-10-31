// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package db

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Router picks the database of a request (PL47, Hauptplan E137, E141). The control database
// holds what belongs to the installation: packages, types with the customer layer, roles and
// permissions, and with PL47b the tenant directory and the accounts of the installation. Each
// tenant may have a database of its own for everything else; without that, one database is
// both, and every method returns the same pool.
//
// Services hold a Router, never a pool (a test guards that): the choice between control and
// tenant is made at each access, so that the operating mode is one switch in the configuration.
type Router struct {
	control *pgxpool.Pool
	tenants map[string]*pgxpool.Pool
	// tenantOf names the tenant of a request (authz: the tenant the session works in).
	tenantOf func(context.Context) string
}

// Single is the router of an installation with one database (the default, and every test that
// does not test the operating modes).
func Single(pool *pgxpool.Pool) *Router { return &Router{control: pool} }

// NewRouter routes the tenants named in tenants to their pools and everything else to the
// control database; tenantOf reads the tenant of a request.
func NewRouter(control *pgxpool.Pool, tenants map[string]*pgxpool.Pool, tenantOf func(context.Context) string) *Router {
	return &Router{control: control, tenants: tenants, tenantOf: tenantOf}
}

// Control is the database of the installation.
func (r *Router) Control() *pgxpool.Pool { return r.control }

// Tenant is the database of the tenant the request works in; the control database for the
// root, for work without a tenant and with one database only. Background work bound to a
// database (Database.Bind: a job, the dispatcher) gets that one, whatever else the context says.
func (r *Router) Tenant(ctx context.Context) *pgxpool.Pool {
	if p, ok := ctx.Value(boundKey{}).(*pgxpool.Pool); ok && p != nil {
		return p
	}
	if len(r.tenants) == 0 || r.tenantOf == nil {
		return r.control
	}
	return r.For(r.tenantOf(ctx))
}

// For is the database of a tenant given by its node; a node below the tenant counts as the
// tenant. The root and an unknown node go to the control database.
func (r *Router) For(tenant string) *pgxpool.Pool {
	if p, ok := r.tenants[tenantNode(tenant)]; ok {
		return p
	}
	return r.control
}

// Separate reports whether tenants have databases of their own.
func (r *Router) Separate() bool { return len(r.tenants) > 0 }

// Pools lists every database once: the control database first, then those of the tenants.
// Migrations, package installation and the workers of the background run on each.
func (r *Router) Pools() []*pgxpool.Pool {
	dbs := r.Databases()
	out := make([]*pgxpool.Pool, len(dbs))
	for i, d := range dbs {
		out[i] = d.pool
	}
	return out
}

// Database is one database of the installation with the name the background work knows it by
// (PL47c): empty for the control database, the node of its tenant otherwise.
type Database struct {
	Name string
	pool *pgxpool.Pool
}

// Pool is the connection pool of the database.
func (d Database) Pool() *pgxpool.Pool { return d.pool }

// Bind binds work outside a request to this database: Router.Tenant returns it from then on.
// Every job is worked bound to the database it was queued in, the dispatcher of a database
// reads that database's events.
func (d Database) Bind(ctx context.Context) context.Context {
	return context.WithValue(ctx, boundKey{}, d.pool)
}

type boundKey struct{}

// Databases lists every database once, as Pools does, with its name; a database that several
// tenants share carries the first of their names.
func (r *Router) Databases() []Database {
	out := []Database{{pool: r.control}}
	for _, t := range slices.Sorted(maps.Keys(r.tenants)) {
		p := r.tenants[t]
		if !slices.ContainsFunc(out, func(d Database) bool { return d.pool == p }) {
			out = append(out, Database{Name: t, pool: p})
		}
	}
	return out
}

// Current is the database Tenant picks for the context, with its name.
func (r *Router) Current(ctx context.Context) Database {
	p := r.Tenant(ctx)
	for _, d := range r.Databases() {
		if d.pool == p {
			return d
		}
	}
	return Database{pool: p}
}

// tenantNode cuts a path to its first two labels: root.a.b → root.a.
func tenantNode(path string) string {
	parts := strings.SplitN(path, ".", 3)
	if len(parts) < 2 {
		return path
	}
	return parts[0] + "." + parts[1]
}
