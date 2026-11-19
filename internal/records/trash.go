// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// TrashEntry is one record in the trash (PL74, Kern-Plan E78).
type TrashEntry struct {
	Type      string     `json:"type"`
	ID        uuid.UUID  `json:"id"`
	Title     string     `json:"title"`
	DeletedAt time.Time  `json:"deleted_at"`
	DeletedBy *uuid.UUID `json:"deleted_by,omitempty"`
	// DeletedByName is the display name of the account that moved the record to the trash.
	DeletedByName *string `json:"deleted_by_name,omitempty"`
}

// TrashOptions narrow the trash: one type, a part of the title, a page after a cursor.
type TrashOptions struct {
	Type  string
	Query string
	Limit int
	After string
}

// TrashPage is one page of the trash, newest first; Next is the cursor of the next page.
type TrashPage struct {
	Items []TrashEntry `json:"items"`
	Next  string       `json:"next,omitempty"`
}

// trashCursor is the place of the last entry of a page: deleted_at, type and id order the trash.
type trashCursor struct {
	at  time.Time
	typ string
	id  uuid.UUID
}

func (c trashCursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.at.Format(time.RFC3339Nano) + "|" + c.typ + "|" + c.id.String()))
}

// errBadCursor answers an after that no page of the trash gave out (422).
var errBadCursor = &ValidationError{Problems: []Problem{{Field: "after", Code: "cursor", Message: "not a cursor of the trash"}}}

func parseTrashCursor(s string) (trashCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return trashCursor{}, errBadCursor
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return trashCursor{}, errBadCursor
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return trashCursor{}, errBadCursor
	}
	id, err := uuid.Parse(parts[2])
	if err != nil {
		return trashCursor{}, errBadCursor
	}
	return trashCursor{at: at, typ: parts[1], id: id}, nil
}

// ListTrash lists the records in the trash over all types, newest first, only what the session
// may read (PL74). One statement: a part per type with its rights, joined with UNION ALL; each
// part reads only its trash through the partial index on deleted_at (schema, PL74). Who moved
// a record to the trash is updated_by: a record in the trash is never written until it is
// restored, which sets updated_by anew (measured against the audit on H14, Kern-Plan PL74).
func (s *Service) ListTrash(ctx context.Context, o TrashOptions) (TrashPage, error) {
	limit := o.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var after *trashCursor
	if o.After != "" {
		c, err := parseTrashCursor(o.After)
		if err != nil {
			return TrashPage{}, err
		}
		after = &c
	}
	types, err := s.Types(ctx)
	if err != nil {
		return TrashPage{}, err
	}
	if o.Type != "" {
		if _, err := s.Type(ctx, o.Type); err != nil {
			return TrashPage{}, err
		}
	}
	tables, err := s.withTables(ctx, types)
	if err != nil {
		return TrashPage{}, err
	}
	a := authz.From(ctx)
	var parts []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	for _, t := range types {
		name := t.QualifiedName()
		if (o.Type != "" && name != o.Type) || !tables[name] || !a.Allowed(name, authz.ActionRead) {
			continue
		}
		title := "id::text"
		if t.TitleField != "" {
			all, fields := a.ReadableFields(name)
			if all || fields[t.TitleField] {
				title = "coalesce(" + pgx.Identifier{t.TitleField}.Sanitize() + "::text, id::text)"
			}
		}
		where := []string{"deleted_at IS NOT NULL"}
		if c := listScope(t, ""); c != "" {
			where = append(where, c)
		}
		var cond string
		cond, args = a.Filter(name, authz.ActionRead, args)
		where = append(where, cond)
		if q := strings.TrimSpace(o.Query); q != "" {
			where = append(where, title+" ILIKE "+arg("%"+escapeLike(q)+"%"))
		}
		typ := arg(name)
		if after != nil {
			where = append(where, fmt.Sprintf("(deleted_at, %s::text, id) < (%s::timestamptz, %s::text, %s::uuid)",
				typ, arg(after.at), arg(after.typ), arg(after.id)))
		}
		parts = append(parts, fmt.Sprintf(
			"(SELECT %s::text AS type, id, %s AS title, deleted_at, updated_by FROM %s WHERE %s ORDER BY deleted_at DESC, id DESC LIMIT %d)",
			typ, title, table(t), strings.Join(where, " AND "), limit+1))
	}
	page := TrashPage{Items: []TrashEntry{}}
	if len(parts) == 0 {
		return page, nil
	}
	// The parts in a FROM of their own: a single part in parentheses would take the outer
	// ORDER BY as a second one of its own, which PostgreSQL refuses.
	sql := "SELECT type, id, title, deleted_at, updated_by FROM (" + strings.Join(parts, " UNION ALL ") +
		fmt.Sprintf(") trash ORDER BY deleted_at DESC, type DESC, id DESC LIMIT %d", limit+1)
	rows, err := s.db.Tenant(ctx).Query(ctx, sql, planned(args)...)
	if err != nil {
		return TrashPage{}, fmt.Errorf("records: trash: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e TrashEntry
		if err := rows.Scan(&e.Type, &e.ID, &e.Title, &e.DeletedAt, &e.DeletedBy); err != nil {
			return TrashPage{}, fmt.Errorf("records: trash: %w", err)
		}
		page.Items = append(page.Items, e)
	}
	if err := rows.Err(); err != nil {
		return TrashPage{}, fmt.Errorf("records: trash: %w", err)
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.Next = trashCursor{at: last.DeletedAt, typ: last.Type, id: last.ID}.String()
	}
	return page, s.trashNames(ctx, page.Items)
}

// withTables names the types whose table exists in the database of the session. A type
// without one (a package whose install stopped halfway; the tests register types without
// tables) would fail a statement over all types (found by the browser test of UI63).
func (s *Service) withTables(ctx context.Context, types []meta.Type) (map[string]bool, error) {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = schema.TableName(t)
	}
	rows, err := s.db.Tenant(ctx).Query(ctx, `SELECT n FROM unnest($1::text[]) n WHERE to_regclass(quote_ident(n)) IS NOT NULL`, names)
	if err != nil {
		return nil, fmt.Errorf("records: tables of the types: %w", err)
	}
	have, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("records: tables of the types: %w", err)
	}
	set := make(map[string]bool, len(have))
	for _, n := range have {
		set[n] = true
	}
	out := make(map[string]bool, len(types))
	for i, t := range types {
		out[t.QualifiedName()] = set[names[i]]
	}
	return out, nil
}

// trashNames fills in the names of the accounts that moved the records to the trash.
func (s *Service) trashNames(ctx context.Context, items []TrashEntry) error {
	var ids []uuid.UUID
	for _, e := range items {
		if e.DeletedBy != nil {
			ids = append(ids, *e.DeletedBy)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := queries.New(s.db.Tenant(ctx)).AccountNames(ctx, ids)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("records: trash names: %w", err)
	}
	names := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		names[r.ID] = r.DisplayName
	}
	for i := range items {
		if b := items[i].DeletedBy; b != nil {
			if n, ok := names[*b]; ok {
				items[i].DeletedByName = &n
			}
		}
	}
	return nil
}
