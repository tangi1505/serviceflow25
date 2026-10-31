// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package query holds the read queries that span type tables (architektur.md §3.1, E34: only
// records and query touch type tables). The first one is the membership of groups (PL7).
package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/db"
)

// ErrNotFound says the group does not exist.
var ErrNotFound = errors.New("query: group not found")

// Member is a person in a group, with the group the membership comes from (a subgroup when
// the query is recursive).
type Member struct {
	PersonID  uuid.UUID `json:"person_id"`
	LastName  string    `json:"last_name"`
	FirstName string    `json:"first_name,omitempty"`
	Email     string    `json:"email,omitempty"`
	GroupID   uuid.UUID `json:"group_id"`
	GroupName string    `json:"group_name"`
}

// Groups answers questions about core.group and core.group_member.
type Groups struct {
	db *db.Router
}

// NewGroups returns the group queries on the pool.
func NewGroups(rt *db.Router) *Groups { return &Groups{db: rt} }

// GroupsOfPerson lists the groups a person is a member of (direct memberships, not trashed);
// the rights of PL8 reach a person through them.
func (g *Groups) GroupsOfPerson(ctx context.Context, personID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := g.db.Tenant(ctx).Query(ctx, `SELECT "group" FROM rec_core_group_member WHERE person = $1 AND deleted_at IS NULL`, personID)
	if err != nil {
		return nil, fmt.Errorf("query: groups of person: %w", err)
	}
	defer rows.Close()
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("query: groups of person: %w", err)
	}
	return ids, nil
}

// NodeOfPerson is the node of a person's record (PL46), empty when it is trashed or gone.
func (g *Groups) NodeOfPerson(ctx context.Context, personID uuid.UUID) (string, error) {
	var node string
	err := g.db.Tenant(ctx).QueryRow(ctx, `SELECT org_path::text FROM rec_core_person WHERE id = $1 AND deleted_at IS NULL`, personID).Scan(&node)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("query: node of person: %w", err)
	}
	return node, nil
}

// AccountsOfGroupMates lists the accounts of every person who shares a group with the person
// (the person included): the scope "group" of a permission (PL8) means records created by one
// of them. The core package may be absent; then only the person's own accounts count.
func (g *Groups) AccountsOfGroupMates(ctx context.Context, personID uuid.UUID) ([]uuid.UUID, error) {
	const sql = `
SELECT a.id FROM account a
WHERE a.person_id = $1
   OR a.person_id IN (
        SELECT m2.person FROM rec_core_group_member m1
        JOIN rec_core_group_member m2 ON m2."group" = m1."group" AND m2.deleted_at IS NULL
        WHERE m1.person = $1 AND m1.deleted_at IS NULL)`
	rows, err := g.db.Tenant(ctx).Query(ctx, sql, personID)
	if err != nil {
		return nil, fmt.Errorf("query: accounts of group mates: %w", err)
	}
	defer rows.Close()
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("query: accounts of group mates: %w", err)
	}
	return ids, nil
}

// Members lists the persons of a group; with recursive, of its subgroups too (parent chain,
// any depth). Trashed groups, members and persons are left out. Order: group, last name,
// first name.
func (g *Groups) Members(ctx context.Context, groupID uuid.UUID, recursive bool) ([]Member, error) {
	var exists bool
	err := g.db.Tenant(ctx).QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM rec_core_group WHERE id = $1 AND deleted_at IS NULL)", groupID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("query: group: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	const sql = `
WITH RECURSIVE tree AS (
    SELECT id, name, 0 AS depth FROM rec_core_group WHERE id = $1 AND deleted_at IS NULL
    UNION ALL
    SELECT c.id, c.name, tree.depth + 1
    FROM rec_core_group c JOIN tree ON c.parent = tree.id
    WHERE c.deleted_at IS NULL AND $2 AND tree.depth < 50
)
SELECT p.id, p.last_name, COALESCE(p.first_name, ''), COALESCE(p.email, ''), tree.id, tree.name
FROM tree
JOIN rec_core_group_member m ON m."group" = tree.id AND m.deleted_at IS NULL
JOIN rec_core_person p ON p.id = m.person AND p.deleted_at IS NULL
ORDER BY tree.depth, tree.name, p.last_name, p.first_name`
	rows, err := g.db.Tenant(ctx).Query(ctx, sql, groupID, recursive)
	if err != nil {
		return nil, fmt.Errorf("query: members: %w", err)
	}
	defer rows.Close()
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		err := row.Scan(&m.PersonID, &m.LastName, &m.FirstName, &m.Email, &m.GroupID, &m.GroupName)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("query: members: %w", err)
	}
	return members, nil
}
