// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
)

// --- one bulk row (PL31; Hauptplan E38; architektur.md §6.4) ---
//
// A source delivers records under its own keys. Sync finds the record by source and external
// key: none yet means create, the same row hash means nothing to do, another hash means
// update the delivered fields. Fields the row does not carry stay as they are. Rights, rules,
// formulas, search, audit and events are those of Create and Update; an unchanged row writes
// nothing at all.

// SyncKey identifies a bulk row: the source, its key for the record, and the checksum of the
// fields the row delivers.
type SyncKey struct {
	Source, Key string
	Hash        []byte
}

// Outcomes of Sync.
const (
	SyncCreated   = "created"
	SyncUpdated   = "updated"
	SyncUnchanged = "unchanged"
)

// Sync writes one bulk row and returns the record's id and version and what happened.
func (s *Service) Sync(ctx context.Context, typeName string, key SyncKey, fields map[string]any, actor Actor) (uuid.UUID, int64, string, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return uuid.Nil, 0, "", err
	}
	// The key is unique per tenant (PL46): the search stays in the tenant of the row's node.
	where := "source = $1 AND external_key = $2 AND tenant = tenant_of($3::ltree)"
	if scope := listScope(t, ""); scope != "" {
		where += " AND " + scope
	}
	var (
		id      uuid.UUID
		version int64
		hash    []byte
		trashed bool
	)
	sql := fmt.Sprintf("SELECT id, version, row_hash, deleted_at IS NOT NULL FROM %s WHERE %s", table(t), where)
	err = s.db.Tenant(ctx).QueryRow(ctx, sql, key.Source, key.Key, syncNode(ctx, fields)).Scan(&id, &version, &hash, &trashed)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		rec, err := s.create(ctx, t, fields, actor, &key, nil)
		if err != nil {
			return uuid.Nil, 0, "", err
		}
		id, err := uuid.Parse(fmt.Sprint(rec["id"]))
		return id, rec["version"].(int64), SyncCreated, err
	case err != nil:
		return uuid.Nil, 0, "", err
	case trashed:
		return id, version, "", fmt.Errorf("%w: the record of %s %q is in the trash; restore it first", ErrConflict, key.Source, key.Key)
	case bytes.Equal(hash, key.Hash):
		// Nothing to write, but the caller may only hear of the record with the right to
		// change it, like for an update.
		rec, err := s.read(ctx, s.db.Tenant(ctx), t, id, false, false)
		if err != nil {
			return uuid.Nil, 0, "", err
		}
		if err := checkAccess(ctx, t, rec, authz.ActionUpdate); err != nil {
			return uuid.Nil, 0, "", err
		}
		return id, version, SyncUnchanged, nil
	}
	// Another hash: the record of this version must still exist; a change meanwhile is a
	// conflict of this row, the caller sends it again.
	rec, err := s.update(ctx, t, id, version, fields, actor, &key, nil)
	if err != nil {
		return id, version, "", err
	}
	v := rec["version"].(int64)
	if v == version {
		// The fields were the same, only the hash was new (first bulk run on a record).
		return id, v, SyncUnchanged, nil
	}
	return id, v, SyncUpdated, nil
}

// storeRowHash keeps the hash of a bulk row whose fields did not change: no version, no audit,
// no event.
func storeRowHash(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, hash []byte) error {
	_, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET row_hash = $2 WHERE id = $1", table(t)), id, hash)
	return err
}
