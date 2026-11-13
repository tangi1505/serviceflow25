// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package seed creates the start data of packages (SD-Plan N9, E6, Hauptplan E160): the
// entries below a package's data/, records and rules, once per tenant, or at the root without
// tenants. The table package_seed remembers each entry a tenant got and what became of it;
// what is there stays as it is, what was deleted does not come back, so the customer owns it.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/rules/engine"
)

// Service creates start data with the records and the rules of the core.
type Service struct {
	db    *db.Router
	rec   *records.Service
	rules *engine.Engine
}

// New wires the service.
func New(rt *db.Router, rec *records.Service, rules *engine.Engine) *Service {
	return &Service{db: rt, rec: rec, rules: rules}
}

// Package creates the start data of p in every tenant, without tenants at the root; it
// returns how many entries it created.
func (s *Service) Package(ctx context.Context, p meta.Package) (int, error) {
	if len(p.Data) == 0 {
		return 0, nil
	}
	scopes, err := s.scopes(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, scope := range scopes {
		n, err := s.Into(ctx, p, scope)
		created += n
		if err != nil {
			return created, err
		}
	}
	return created, nil
}

// Installed creates the start data of every installed package at the node, a new tenant, the
// packages in the order of their dependencies.
func (s *Service) Installed(ctx context.Context, scope string) (int, error) {
	rows, err := queries.New(s.db.Control()).ListPackages(ctx)
	if err != nil {
		return 0, fmt.Errorf("seed: packages: %w", err)
	}
	byName := map[string]meta.Package{}
	for _, row := range rows {
		var p meta.Package
		if err := json.Unmarshal(row.Spec, &p); err != nil {
			return 0, fmt.Errorf("seed: package %s: %w", row.Name, err)
		}
		byName[p.Name] = p
	}
	created := 0
	for _, name := range ordered(byName) {
		n, err := s.Into(ctx, byName[name], scope)
		created += n
		if err != nil {
			return created, err
		}
	}
	return created, nil
}

// ordered sorts the packages so that each comes after those it depends on.
func ordered(byName map[string]meta.Package) []string {
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	var out []string
	done := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		p, ok := byName[name]
		if !ok || done[name] {
			return
		}
		done[name] = true
		for _, d := range p.Depends {
			visit(d)
		}
		out = append(out, name)
	}
	for _, name := range names {
		visit(name)
	}
	return out
}

// scopes are the tenants, or the root without tenants.
func (s *Service) scopes(ctx context.Context) ([]string, error) {
	tenants, err := queries.New(s.db.Control()).ListTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: tenants: %w", err)
	}
	if len(tenants) == 0 {
		return []string{org.Root}, nil
	}
	out := make([]string, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, t.Path)
	}
	return out, nil
}

// Into creates the entries of p at the node that it never created there, in their order, with
// the system access of the node and without an actor.
func (s *Service) Into(ctx context.Context, p meta.Package, scope string) (int, error) {
	if len(p.Data) == 0 {
		return 0, nil
	}
	ctx = s.db.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	pool := s.db.Tenant(ctx)
	ids, err := known(ctx, pool, scope)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, e := range p.Data {
		if _, done := ids[p.Name+"."+e.Key]; done {
			continue
		}
		target, ref, err := s.create(ctx, p.Name, e, scope, ids)
		if err != nil {
			return created, fmt.Errorf("seed: %s.%s at %s: %w", p.Name, e.Key, scope, err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO package_seed (org_path, package, key, target, ref)
VALUES ($1::ltree, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, scope, p.Name, e.Key, target, ref); err != nil {
			return created, fmt.Errorf("seed: %s.%s at %s: %w", p.Name, e.Key, scope, err)
		}
		ids[p.Name+"."+e.Key] = ref
		created++
	}
	return created, nil
}

// Refresh puts the rules of the start data with the given keys again as the package defines
// them, at the node, over the rules made before: an upgrade step of the package decides when
// (WF7: the rules that asked from the stages of request 0.2). It returns how many it put.
func (s *Service) Refresh(ctx context.Context, p meta.Package, scope string, keys ...string) (int, error) {
	ctx = s.db.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	ids, err := known(ctx, s.db.Tenant(ctx), scope)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range p.Data {
		if e.Rule == nil || !slices.Contains(keys, e.Key) {
			continue
		}
		if _, _, err := s.create(ctx, p.Name, e, scope, ids); err != nil {
			return n, fmt.Errorf("seed: %s.%s at %s: %w", p.Name, e.Key, scope, err)
		}
		n++
	}
	return n, nil
}

// Ref is what the start data entry pkg.key became at the node: the id of its record or the
// name of its rule; empty when the node never got it (an upgrade step finds it so, SD20).
func (s *Service) Ref(ctx context.Context, scope, pkg, key string) (string, error) {
	ctx = s.db.BindFor(authz.WithAccess(ctx, authz.SystemIn(scope)), scope)
	ids, err := known(ctx, s.db.Tenant(ctx), scope)
	if err != nil {
		return "", err
	}
	return ids[pkg+"."+key], nil
}

// Scopes are the tenants, or the root without tenants.
func (s *Service) Scopes(ctx context.Context) ([]string, error) { return s.scopes(ctx) }

// known is what the node got so far, "package.key" to a record id or a rule name.
func known(ctx context.Context, pool *pgxpool.Pool, scope string) (map[string]string, error) {
	rows, err := pool.Query(ctx, "SELECT package, key, ref FROM package_seed WHERE org_path = $1::ltree", scope)
	if err != nil {
		return nil, fmt.Errorf("seed: known: %w", err)
	}
	defer rows.Close()
	ids := map[string]string{}
	for rows.Next() {
		var pkg, key, ref string
		if err := rows.Scan(&pkg, &key, &ref); err != nil {
			return nil, fmt.Errorf("seed: known: %w", err)
		}
		ids[pkg+"."+key] = ref
	}
	return ids, rows.Err()
}

// create makes one entry: a record of its type at the node, or a rule of the node's tenant.
func (s *Service) create(ctx context.Context, pkg string, e meta.Seed, scope string, ids map[string]string) (string, string, error) {
	if e.Rule != nil {
		raw, err := resolve(e.Rule, pkg, ids)
		if err != nil {
			return "", "", err
		}
		if _, set := raw["enabled"]; !set {
			raw["enabled"] = true
		}
		var r engine.Rule
		if err := remarshal(raw, &r); err != nil {
			return "", "", err
		}
		name := RuleName(pkg, e.Key, scope)
		if len(name) > 63 {
			return "", "", fmt.Errorf("rule name %s is longer than 63 characters", name)
		}
		if _, _, err := s.rules.Put(ctx, name, r, false, nil); err != nil {
			return "", "", err
		}
		return "rule", name, nil
	}
	values, err := resolve(e.Values, pkg, ids)
	if err != nil {
		return "", "", err
	}
	// A reference to a select list may name the key of its entry, as fixed values of forms do
	// (SD13: the area of an offer in the catalog).
	t, err := s.rec.Type(ctx, e.Type)
	if err != nil {
		return "", "", err
	}
	if values, err = s.rec.ListKeys(ctx, t, values); err != nil {
		return "", "", err
	}
	values["org_path"] = scope
	rec, err := s.rec.Create(ctx, e.Type, values, nil)
	if err != nil {
		return "", "", err
	}
	return e.Type, fmt.Sprint(rec["id"]), nil
}

// RuleName is the name of a rule of the start data: package and key, in a tenant with its name,
// because rule names are unique over all tenants.
func RuleName(pkg, key, scope string) string {
	name := pkg + "_" + key
	if scope != org.Root {
		name += "_" + scope[strings.LastIndex(scope, ".")+1:]
	}
	return name
}

// resolve replaces the "@" names and gives the values as JSON would: numbers as float64.
func resolve(v map[string]any, pkg string, ids map[string]string) (map[string]any, error) {
	raw, err := meta.ResolveSeedRefs(v, pkg, ids)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := remarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func remarshal(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
