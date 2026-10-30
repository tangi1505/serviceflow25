// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package tenant sets up tenants (PL44, Hauptplan E136, architektur.md §5.1). While the
// installation has none, the root is the only tenant. The first tenant can take over all that
// is there (adopt): the tree, the records, the assignments, the login rules and directories
// move below it, and every account is bound to it except the named administrators, who stay
// accounts of the installation. Accounts are bound to a tenant or belong to the installation.
package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// ErrInvalid says a name, a tenant or an account does not fit.
var ErrInvalid = errors.New("tenant: invalid")

// Path turns a tenant name or path into its path: "a" and "root.a" both give "root.a".
func Path(name string) string {
	if strings.HasPrefix(name, org.Root+".") {
		return name
	}
	return org.Root + "." + name
}

// Options of a new tenant.
type Options struct {
	Name  string
	Label map[string]string
	// Adopt moves all that is there into the tenant; only for the first one.
	Adopt bool
	// Admins are usernames that stay accounts of the installation with their roles at the
	// root when adopting.
	Admins []string
}

// Result says what a new tenant took over.
type Result struct {
	Tenant      org.Node
	Nodes       int64
	Records     map[string]int64
	Assignments int64
	Accounts    int64
	// Seeded counts the select list entries a new tenant got as its copy.
	Seeded int
}

// Create adds a tenant; with Adopt the first one takes over all that is there, in one
// transaction.
func Create(ctx context.Context, pool *pgxpool.Pool, o Options) (Result, error) {
	svc := org.New(pool)
	if !o.Adopt {
		if len(o.Admins) > 0 {
			return Result{}, fmt.Errorf("%w: --admin only goes with --adopt", ErrInvalid)
		}
		n, err := svc.CreateTenant(ctx, o.Name, o.Label)
		if err != nil {
			return Result{}, err
		}
		// The new tenant gets its copy of the select lists of every package (PL45, V28).
		types, err := meta.Types(ctx, queries.New(pool))
		if err != nil {
			return Result{Tenant: n}, err
		}
		seeded, err := records.New(pool).SeedListsInto(ctx, types, n.Path)
		return Result{Tenant: n, Seeded: seeded}, err
	}
	tenants, err := svc.Tenants(ctx)
	if err != nil {
		return Result{}, err
	}
	if len(tenants) > 0 {
		return Result{}, fmt.Errorf("%w: only the first tenant can adopt; there is %s", ErrInvalid, tenants[0].Path)
	}
	q := queries.New(pool)
	for _, name := range o.Admins {
		if _, err := q.GetAccountByUsername(ctx, name); err != nil {
			return Result{}, fmt.Errorf("%w: administrator %q: %w", ErrInvalid, name, err)
		}
	}
	types, err := meta.Types(ctx, q)
	if err != nil {
		return Result{}, err
	}
	path := Path(o.Name)
	if !org.ValidPath(path) || strings.Count(path, ".") != 1 {
		return Result{}, fmt.Errorf("%w: tenant name %q must be snake_case (a-z, 0-9, _)", ErrInvalid, o.Name)
	}
	var res Result
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The tenant goes in by hand, in the transaction: CreateTenant refuses a root with other
	// nodes below, and moving those is what adopting is about.
	label := o.Label
	if label == nil {
		label = map[string]string{}
	}
	raw, _ := json.Marshal(label)
	row, err := queries.New(tx).InsertOrgNode(ctx, queries.InsertOrgNodeParams{Path: path, Name: o.Name, Label: raw, Tenant: true})
	if err != nil {
		return Result{}, fmt.Errorf("%w: tenant %s: %w", ErrInvalid, path, err)
	}
	res.Tenant = org.Node{Path: row.Path, Name: row.Name, Label: label, Parent: org.Root, Tenant: true}
	moved := records.MovedPath("path")
	admins := o.Admins
	if admins == nil {
		admins = []string{}
	}
	// Every step gets the tenant as $1; those marked admins the administrators as $2.
	steps := []struct {
		sql    string
		n      *int64
		admins bool
	}{
		// The tree: copies below the tenant, the settings onto them, then away with the old.
		{"INSERT INTO org_node (path, name, label, created_at, updated_at) SELECT " + moved +
			", name, label, created_at, updated_at FROM org_node WHERE nlevel(path) > 1 AND NOT path <@ $1::ltree", &res.Nodes, false},
		{"UPDATE org_setting SET path = " + moved + " WHERE nlevel(path) > 1 AND NOT path <@ $1::ltree", nil, false},
		{"DELETE FROM org_node WHERE nlevel(path) > 1 AND NOT path <@ $1::ltree", nil, false},
		// Roles further down move with the tree; those at the root move into the tenant,
		// except the administrators' ($2).
		{"UPDATE role_assignment SET org_path = " + records.MovedPath("org_path") + " WHERE nlevel(org_path) > 1 OR person_id IS NULL" +
			" OR person_id NOT IN (SELECT person_id FROM account WHERE username = ANY($2::text[]) AND person_id IS NOT NULL)", &res.Assignments, true},
		{"UPDATE login_rule SET org_path = " + records.MovedPath("org_path"), nil, false},
		// Rules and webhooks of before belong to the first tenant (PL45).
		{"UPDATE rule SET tenant = $1::ltree WHERE tenant IS NULL", nil, false},
		{"UPDATE webhook SET tenant = $1::ltree WHERE tenant = 'root'", nil, false},
		{"UPDATE ldap_directory SET org_path = " + records.MovedPath("org_path"), nil, false},
		{"UPDATE account SET tenant = $1::ltree, updated_at = now() WHERE tenant IS NULL AND NOT username = ANY($2::text[])", &res.Accounts, true},
	}
	for _, st := range steps {
		args := []any{path}
		if st.admins {
			args = append(args, admins)
		}
		tag, err := tx.Exec(ctx, st.sql, args...)
		if err != nil {
			return Result{}, fmt.Errorf("tenant: adopt: %w", err)
		}
		if st.n != nil {
			*st.n = tag.RowsAffected()
		}
	}
	if res.Records, err = records.MoveIntoTenant(ctx, tx, types, path); err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

// Bind binds an account to a tenant; an empty tenant makes it an account of the installation.
// The person of a bound account must lie in its tenant.
func Bind(ctx context.Context, pool *pgxpool.Pool, username, tenant string) error {
	q := queries.New(pool)
	acc, err := q.GetAccountByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("%w: account %q: %w", ErrInvalid, username, err)
	}
	var t pgtype.Text
	if tenant != "" {
		path := Path(tenant)
		if err := isTenant(ctx, pool, path); err != nil {
			return err
		}
		if acc.PersonID.Valid {
			if err := personIn(ctx, pool, acc.PersonID.UUID, path); err != nil {
				return err
			}
		}
		t = pgtype.Text{String: path, Valid: true}
	}
	return q.SetAccountTenant(ctx, queries.SetAccountTenantParams{ID: acc.ID, Tenant: t})
}

func isTenant(ctx context.Context, pool *pgxpool.Pool, path string) error {
	n, err := org.New(pool).Node(ctx, path)
	if err != nil {
		return fmt.Errorf("%w: tenant %s: %w", ErrInvalid, path, err)
	}
	if !n.Tenant {
		return fmt.Errorf("%w: %s is no tenant", ErrInvalid, path)
	}
	return nil
}

func personIn(ctx context.Context, pool *pgxpool.Pool, person uuid.UUID, tenant string) error {
	p, err := records.New(pool).Get(ctx, "core.person", person)
	if err != nil {
		return fmt.Errorf("tenant: person of the account: %w", err)
	}
	if path, _ := p["org_path"].(string); path != tenant && !strings.HasPrefix(path, tenant+".") {
		return fmt.Errorf("%w: the person of the account lies in %s, not in the tenant %s", ErrInvalid, path, tenant)
	}
	return nil
}
