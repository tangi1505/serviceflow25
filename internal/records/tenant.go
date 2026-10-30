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

	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// MovedPath is the SQL that moves a path of the tree into a tenant ($1): the root becomes the
// tenant, root.x becomes <tenant>.x (PL44).
func MovedPath(column string) string {
	return fmt.Sprintf("CASE WHEN nlevel(%[1]s) = 1 THEN $1::ltree ELSE $1::ltree || subpath(%[1]s, 1) END", column)
}

// MoveIntoTenant moves the records of the types into the tenant, inside the caller's
// transaction (PL44, itsm tenant create --adopt), select list entries included: they become
// the copy of the first tenant (PL45, V28). The history is not written: the records do not
// change, the tree below them does. It returns the rows moved per type.
func MoveIntoTenant(ctx context.Context, tx pgx.Tx, types []meta.Type, tenant string) (map[string]int64, error) {
	moved := map[string]int64{}
	lists := false
	for _, t := range types {
		if t.IsList() {
			lists = true
			continue
		}
		n, err := moveRows(ctx, tx, schema.TableName(t), tenant)
		if err != nil {
			return nil, fmt.Errorf("records: move %s: %w", t.QualifiedName(), err)
		}
		moved[t.QualifiedName()] = n
	}
	if lists {
		n, err := moveRows(ctx, tx, schema.ListTable, tenant)
		if err != nil {
			return nil, fmt.Errorf("records: move list entries: %w", err)
		}
		moved[schema.ListTable] = n
	}
	return moved, nil
}

func moveRows(ctx context.Context, tx pgx.Tx, name, tenant string) (int64, error) {
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil || !exists {
		return 0, err
	}
	tag, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET org_path = %s", pgx.Identifier{name}.Sanitize(), MovedPath("org_path")), tenant)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
