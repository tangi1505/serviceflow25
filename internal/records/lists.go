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
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// --- select lists (architektur.md B3; Hauptplan E93; PL16) ---
//
// A list is a type with the trait "list". Its entries share the core table list_entry, so
// every statement on a list carries the condition list = '<package.type>' (listScope). A tree
// keeps path (ltree of ids) and full_name up to date: treeRefresh recomputes the entry and
// its descendants after an insert or a change of name or parent; checkTreeParent refuses a
// parent below the entry itself.

// fullNameColumn is the derived column of a tree list the API shows next to the fields.
const fullNameColumn = "full_name"

// listScope is the condition that keeps a statement within one list, or "" for a type with
// a table of its own. The name is a validated identifier, so it is safe as a literal.
func listScope(t meta.Type, alias string) string {
	if !t.IsList() {
		return ""
	}
	col := "list"
	if alias != "" {
		col = alias + ".list"
	}
	return fmt.Sprintf("%s = '%s'", col, strings.ReplaceAll(t.QualifiedName(), "'", "''"))
}

// extraColumns are the derived columns of a type beyond its fields: the full name of a tree
// list, the roles at the record (WF3).
func extraColumns(t meta.Type) []string {
	var out []string
	if t.IsTree() {
		out = append(out, fullNameColumn)
	}
	if hasActors(t) {
		out = append(out, actorsField)
	}
	return out
}

// isExtra reports whether name is a derived column of the type.
func isExtra(t meta.Type, name string) bool {
	for _, c := range extraColumns(t) {
		if c == name {
			return true
		}
	}
	return false
}

// treeSQL recomputes path and full_name of one entry and everything below it.
const treeSQL = `WITH RECURSIVE sub AS (
  SELECT l.id,
         COALESCE((SELECT p.path FROM list_entry p WHERE p.id = l.parent), ''::ltree) || replace(l.id::text, '-', '')::ltree AS path,
         COALESCE((SELECT p.full_name || ' > ' FROM list_entry p WHERE p.id = l.parent), '') || l.name AS full_name
  FROM list_entry l WHERE l.id = $1
  UNION ALL
  SELECT c.id, sub.path || replace(c.id::text, '-', '')::ltree, sub.full_name || ' > ' || c.name
  FROM list_entry c JOIN sub ON c.parent = sub.id
)
UPDATE list_entry l SET path = sub.path, full_name = sub.full_name FROM sub WHERE l.id = sub.id`

func treeRefresh(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	if _, err := tx.Exec(ctx, treeSQL, id); err != nil {
		return fmt.Errorf("records: tree: %w", err)
	}
	return nil
}

// checkTreeParent refuses a new parent that is the entry itself or one of its descendants.
func checkTreeParent(ctx context.Context, tx pgx.Tx, id uuid.UUID, values map[string]any) error {
	parent, ok := values["parent"].(uuid.UUID)
	if !ok {
		return nil
	}
	var cycle bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM list_entry WHERE id = $1 AND path <@ (SELECT path FROM list_entry WHERE id = $2))", parent, id).Scan(&cycle)
	if err != nil {
		return fmt.Errorf("records: tree: %w", err)
	}
	if cycle {
		return &ValidationError{Problems: []Problem{{Field: "parent", Code: "cycle", Message: "must not be the entry itself or one below it"}}}
	}
	return nil
}

// treeChanged reports whether a change touches what path and full_name are made of.
func treeChanged(changes []change) bool {
	for _, c := range changes {
		if c.field == "name" || c.field == "parent" {
			return true
		}
	}
	return false
}

// underCondition keeps the entries below the given one (not the entry itself).
func underCondition(ref string) string {
	return fmt.Sprintf("(path <@ (SELECT path FROM list_entry WHERE id = %s) AND id <> %s)", ref, ref)
}

// SeedLists creates the entries a package ships for its lists (meta.ListSpec.Entries) in
// every tenant, each its own copy (PL45, V28); without tenants at the root.
func (s *Service) SeedLists(ctx context.Context, p meta.Package) (int, error) {
	tenants, err := queries.New(s.db.Control()).ListTenants(ctx)
	if err != nil {
		return 0, fmt.Errorf("records: tenants: %w", err)
	}
	scopes := []string{"root"}
	if len(tenants) > 0 {
		scopes = scopes[:0]
		for _, t := range tenants {
			scopes = append(scopes, t.Path)
		}
	}
	created := 0
	for _, scope := range scopes {
		n, err := s.SeedListsInto(ctx, p.Types, scope)
		created += n
		if err != nil {
			return created, err
		}
	}
	return created, nil
}

// SeedListsInto creates the shipped entries of the list types at the node, a tenant or the
// root: an entry whose key exists there is left as it is, a missing one is created with the
// system access and without an actor. Parents are resolved by key.
func (s *Service) SeedListsInto(ctx context.Context, types []meta.Type, scope string) (int, error) {
	// The copy goes into the database of the scope's tenant (PL47d), whatever the caller works in.
	ctx = s.db.BindFor(authz.WithAccess(ctx, authz.System()), scope)
	created := 0
	for _, t := range types {
		if !t.IsList() || t.List == nil {
			continue
		}
		ids := map[string]uuid.UUID{}
		for _, e := range t.List.Entries {
			var id uuid.UUID
			err := s.db.Tenant(ctx).QueryRow(ctx, fmt.Sprintf("SELECT id FROM %s WHERE list = $1 AND key = $2 AND deleted_at IS NULL AND org_path <@ $3::ltree", schema.ListTable),
				t.QualifiedName(), e.Key, scope).Scan(&id)
			switch {
			case err == nil:
				ids[e.Key] = id
				continue
			case !errors.Is(err, pgx.ErrNoRows):
				return created, fmt.Errorf("records: seed %s: %w", t.QualifiedName(), err)
			}
			input := map[string]any{"name": e.Name, "key": e.Key, "org_path": scope}
			if e.Description != "" {
				input["description"] = e.Description
			}
			if e.Position != nil {
				input["position"] = *e.Position
			}
			if e.Parent != "" {
				parent, ok := ids[e.Parent]
				if !ok {
					return created, fmt.Errorf("records: seed %s: entry %s names parent %q, which is not seeded", t.QualifiedName(), e.Key, e.Parent)
				}
				input["parent"] = parent.String()
			}
			for name, v := range map[string]*float64{"lat": e.Lat, "lon": e.Lon, "alt": e.Alt} {
				if v != nil {
					input[name] = *v
				}
			}
			rec, err := s.Create(ctx, t.QualifiedName(), input, nil)
			if err != nil {
				return created, fmt.Errorf("records: seed %s entry %s: %w", t.QualifiedName(), e.Key, err)
			}
			ids[e.Key] = uuid.MustParse(rec["id"].(string))
			created++
		}
	}
	return created, nil
}
