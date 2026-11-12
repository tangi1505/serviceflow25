// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// RemovedRows reads the records of a type its package gave up: the definition is gone, the
// table and the roles stay as they were (WF7: the stages of request 0.2, which an upgrade step
// moves). Each record has its columns as JSON gives them (numbers as float64) and the holders
// of its roles under "actors"; records in the trash and outside the node scope are left out.
// Without the table there are none.
func (s *Service) RemovedRows(ctx context.Context, pkg, name, scope string) ([]Record, error) {
	t := meta.Type{Package: pkg, Name: name}
	tbl := schema.TableName(t)
	pool := s.db.Tenant(ctx)
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, tbl).Scan(&exists); err != nil {
		return nil, fmt.Errorf("records: removed type %s: %w", t.QualifiedName(), err)
	}
	if !exists {
		return nil, nil
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT to_jsonb(x) FROM %s x WHERE x.deleted_at IS NULL AND x.org_path <@ $1::ltree ORDER BY x.id`,
		pgx.Identifier{tbl}.Sanitize()), scope)
	if err != nil {
		return nil, fmt.Errorf("records: removed type %s: %w", t.QualifiedName(), err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Record, error) {
		var raw []byte
		if err := row.Scan(&raw); err != nil {
			return nil, err
		}
		rec := Record{}
		return rec, json.Unmarshal(raw, &rec)
	})
	if err != nil {
		return nil, fmt.Errorf("records: removed type %s: %w", t.QualifiedName(), err)
	}
	if len(list) == 0 {
		return list, nil
	}
	ids := make([]uuid.UUID, 0, len(list))
	at := map[uuid.UUID]Record{}
	for _, rec := range list {
		id, err := uuid.Parse(fmt.Sprint(rec["id"]))
		if err != nil {
			continue
		}
		ids = append(ids, id)
		at[id] = rec
		rec[actorsField] = map[string]any{}
	}
	rows, err = pool.Query(ctx, `SELECT record_id, role, target_type, target_id FROM actor
WHERE type_name = $1 AND record_id = ANY($2::uuid[]) ORDER BY record_id, role, target_type, target_id`, t.QualifiedName(), ids)
	if err != nil {
		return nil, fmt.Errorf("records: removed type %s: %w", t.QualifiedName(), err)
	}
	defer rows.Close()
	for rows.Next() {
		var rid, target uuid.UUID
		var role, typ string
		if err := rows.Scan(&rid, &role, &typ, &target); err != nil {
			return nil, err
		}
		all := at[rid][actorsField].(map[string]any)
		holders, _ := all[role].([]any)
		all[role] = append(holders, map[string]any{"type": typ, "id": target.String()})
	}
	return list, rows.Err()
}
