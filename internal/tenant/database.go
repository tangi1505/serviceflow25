// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
)

// CreateIn adds a tenant with a database of its own (PL47d, Hauptplan E144). The operator has
// made the empty database and set its URL (your decision of 2026-10-30: itsm needs no right to
// create databases). CreateIn migrates it, gives it the tables of every installed type, its
// node and its copy of the select lists, and enters the tenant in the directory of the control
// database last: until then the installation does not know the tenant, so a create that fails
// half way leaves nothing that stops the start, and running it again goes on where it stopped.
// A new tenant in its own database starts empty; adopting moves data within one database only.
func CreateIn(ctx context.Context, control *pgxpool.Pool, url string, o Options, logger *slog.Logger) (Result, error) {
	if o.Adopt || len(o.Admins) > 0 {
		return Result{}, fmt.Errorf("%w: with a database per tenant a new tenant starts empty; --adopt moves data within one database only", ErrInvalid)
	}
	path := Path(o.Name)
	if !org.ValidPath(path) || strings.Count(path, ".") != 1 {
		return Result{}, fmt.Errorf("%w: tenant name %q must be snake_case (a-z, 0-9, _)", ErrInvalid, o.Name)
	}
	if url == "" {
		return Result{}, fmt.Errorf("%w: %s", db.ErrNoDatabase, path)
	}
	if err := freeName(ctx, control, path); err != nil {
		return Result{}, err
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if err := db.Migrate(ctx, url, logger); err != nil {
		return Result{}, fmt.Errorf("tenant %s: %w", path, err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		return Result{}, fmt.Errorf("tenant %s: %w", path, err)
	}
	defer pool.Close()
	if err := emptyFor(ctx, pool, path); err != nil {
		return Result{}, err
	}
	types, err := meta.Types(ctx, queries.New(control))
	if err != nil {
		return Result{}, err
	}
	if _, err := schema.Apply(ctx, pool, types, logger); err != nil {
		return Result{}, fmt.Errorf("tenant %s: tables: %w", path, err)
	}
	label := o.Label
	if label == nil {
		label = map[string]string{}
	}
	raw, _ := json.Marshal(label)
	node := queries.InsertOrgNodeParams{Path: path, Name: o.Name, Label: raw, Tenant: true}
	if _, err := queries.New(pool).GetOrgNode(ctx, path); errors.Is(err, pgx.ErrNoRows) {
		if _, err := queries.New(pool).InsertOrgNode(ctx, node); err != nil {
			return Result{}, fmt.Errorf("tenant %s: node: %w", path, err)
		}
	} else if err != nil {
		return Result{}, err
	}
	rt := db.NewRouter(control, map[string]*pgxpool.Pool{path: pool}, nil)
	seeded, err := records.New(rt).SeedListsInto(ctx, types, path)
	if err != nil {
		return Result{}, err
	}
	started, err := startData(ctx, rt, path)
	if err != nil {
		return Result{}, err
	}
	row, err := queries.New(control).InsertOrgNode(ctx, node)
	if err != nil {
		return Result{}, fmt.Errorf("tenant %s: directory: %w", path, err)
	}
	return Result{Tenant: org.Node{Path: row.Path, Name: row.Name, Label: label, Parent: org.Root, Tenant: true}, Seeded: seeded, Started: started}, nil
}

// freeName checks the directory: the name is not taken, and the root has no nodes below it
// that are no tenant (they would have to be adopted first).
func freeName(ctx context.Context, control *pgxpool.Pool, path string) error {
	nodes, err := org.New(db.Single(control)).Nodes(ctx)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.Path == path {
			return fmt.Errorf("%w: %s", org.ErrExists, path)
		}
		if n.Parent == org.Root && !n.Tenant {
			return fmt.Errorf("%w: %s below the root is no tenant; move the tree into the first tenant with --adopt", ErrInvalid, n.Path)
		}
	}
	return nil
}

// emptyFor refuses a database that holds more than a new tenant: other nodes than the root
// and the tenant's own (left by an earlier attempt), or accounts.
func emptyFor(ctx context.Context, pool *pgxpool.Pool, path string) error {
	var nodes, accounts int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM org_node WHERE path <> 'root' AND NOT path <@ $1::ltree), (SELECT count(*) FROM account)", path).
		Scan(&nodes, &accounts); err != nil {
		return fmt.Errorf("tenant %s: %w", path, err)
	}
	if nodes > 0 || accounts > 0 {
		return fmt.Errorf("%w: the database of %s is not empty (%d nodes of other tenants, %d accounts)", ErrInvalid, path, nodes, accounts)
	}
	return nil
}

// Finding is one thing Check found wrong.
type Finding struct {
	What   string
	Detail string
}

// Check compares the database of a tenant with the control database (PL47d), for example after
// restoring it from a backup: both stand at the same migration; the tenant is in the directory
// and in its database; every account of the tenant's database is in the central directory of
// usernames under this tenant, and every name the directory gives the tenant has its account
// there; a mirror of an administrator (an account of no tenant, PL47b) has its account in the
// control database. Nothing is changed; the findings say what to do.
func Check(ctx context.Context, control, pool *pgxpool.Pool, path string) ([]Finding, error) {
	var out []Finding
	want, err := db.Version(ctx, control)
	if err != nil {
		return nil, err
	}
	got, err := db.Version(ctx, pool)
	if err != nil {
		return nil, err
	}
	if got != want {
		out = append(out, Finding{"migration", fmt.Sprintf("the tenant's database stands at %d, the control database at %d; run itsm migrate", got, want)})
	}
	for i, q := range []*queries.Queries{queries.New(control), queries.New(pool)} {
		where := []string{"directory", "tenant database"}[i]
		n, err := q.GetOrgNode(ctx, path)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			out = append(out, Finding{"node", fmt.Sprintf("%s is missing in the %s", path, where)})
		case err != nil:
			return nil, err
		case !n.Tenant:
			out = append(out, Finding{"node", fmt.Sprintf("%s is no tenant in the %s", path, where)})
		}
	}
	local, mirrors := map[string]string{}, map[string]string{}
	rows, err := pool.Query(ctx, "SELECT id::text, username, tenant IS NULL FROM account")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name string
		var mirror bool
		if err := rows.Scan(&id, &name, &mirror); err != nil {
			rows.Close()
			return nil, err
		}
		if mirror {
			mirrors[name] = id
		} else {
			local[name] = id
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	central := map[string]string{}
	rows, err = control.Query(ctx, "SELECT account_id::text, username FROM account_home WHERE tenant = $1::ltree", path)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		central[name] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for name, id := range local {
		switch cid, ok := central[name]; {
		case !ok:
			out = append(out, Finding{"account", fmt.Sprintf("%s is in the tenant's database but not in the directory under %s", name, path)})
		case cid != id:
			out = append(out, Finding{"account", fmt.Sprintf("%s has the id %s in the tenant's database and %s in the directory", name, id, cid)})
		}
	}
	for name := range central {
		if _, ok := local[name]; !ok {
			out = append(out, Finding{"account", fmt.Sprintf("the directory gives %s to %s, but the tenant's database has no such account", name, path)})
		}
	}
	for name, id := range mirrors {
		var n int
		if err := control.QueryRow(ctx, "SELECT count(*) FROM account WHERE id = $1::uuid AND tenant IS NULL", id).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			out = append(out, Finding{"mirror", fmt.Sprintf("%s mirrors no administrator of the installation", name)})
		}
	}
	return out, nil
}

// IsTenant checks that a path is a tenant of the directory.
func IsTenant(ctx context.Context, control *pgxpool.Pool, path string) error {
	return isTenant(ctx, control, path)
}
