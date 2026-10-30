// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package schema creates and changes the table of each entity type (architektur.md §3,
// Hauptplan E33): rec_<package>_<type> with the system columns of §3.1 and one column per
// field. Changes run online, under one advisory lock, and are recorded in schema_change.
//
// This is the only package besides db that is allowed to issue DDL.
package schema

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// lockKey serialises schema changes across all instances.
const lockKey = "itsm.schema"

// TableName of a type: rec_<package>_<type>. Select lists (trait list, PL16) share the core
// table list_entry, which the migrations create; Apply leaves them alone.
func TableName(t meta.Type) string {
	if t.IsList() {
		return ListTable
	}
	return "rec_" + t.Package + "_" + t.Name
}

// ListTable holds the entries of every select list (architektur.md B3).
const ListTable = "list_entry"

// systemColumns in creation order (architektur.md §3.1).
var systemColumns = []struct{ name, def string }{
	{"id", "uuid PRIMARY KEY"},
	{"org_path", "ltree NOT NULL DEFAULT 'root'"},
	{"version", "bigint NOT NULL DEFAULT 1"},
	{"state", "text"},
	{"source", "text"},
	{"external_key", "text"},
	{"row_hash", "bytea"},
	{"search", "tsvector"},
	{"created_at", "timestamptz NOT NULL DEFAULT now()"},
	{"created_by", "uuid"},
	{"updated_at", "timestamptz NOT NULL DEFAULT now()"},
	{"updated_by", "uuid"},
	{"deleted_at", "timestamptz"},
}

// ColumnType maps a field type to its PostgreSQL column type.
func ColumnType(ft meta.FieldType) (string, error) {
	switch ft {
	case meta.FieldText, meta.FieldChoice:
		return "text", nil
	case meta.FieldNumber:
		return "numeric", nil
	case meta.FieldBoolean:
		return "boolean", nil
	case meta.FieldDate:
		return "date", nil
	case meta.FieldDateTime:
		return "timestamptz", nil
	case meta.FieldReference:
		return "uuid", nil
	}
	return "", fmt.Errorf("schema: no column type for field type %q", ft)
}

// udtNames is what information_schema reports for each column type.
var udtNames = map[string]string{
	"text": "text", "numeric": "numeric", "boolean": "bool", "date": "date", "timestamptz": "timestamptz", "uuid": "uuid",
}

// Change is one applied DDL step.
type Change struct {
	Type      string
	Change    string
	Statement string
}

// Apply brings the tables of the given types up to their definitions: missing tables are
// created, missing columns added, unique indexes ensured. It returns what it changed.
func Apply(ctx context.Context, pool *pgxpool.Pool, types []meta.Type, logger *slog.Logger) ([]Change, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("schema: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lockKey); err != nil {
		return nil, fmt.Errorf("schema: lock: %w", err)
	}
	q := queries.New(tx)
	var all []Change
	for _, t := range types {
		if t.IsList() {
			continue
		}
		changes, err := applyType(ctx, tx, q, t)
		if err != nil {
			return nil, err
		}
		for _, c := range changes {
			id, err := uuid.NewV7()
			if err != nil {
				return nil, err
			}
			if err := q.InsertSchemaChange(ctx, queries.InsertSchemaChangeParams{
				ID: id, TypeName: t.QualifiedName(), Change: c.Change, Statement: c.Statement,
			}); err != nil {
				return nil, fmt.Errorf("schema: record change: %w", err)
			}
			logger.Info("schema change", "type", t.QualifiedName(), "change", c.Change)
		}
		all = append(all, changes...)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("schema: commit: %w", err)
	}
	return all, nil
}

func applyType(ctx context.Context, tx pgx.Tx, q *queries.Queries, t meta.Type) ([]Change, error) {
	table := TableName(t)
	var changes []Change
	run := func(change, stmt string) error {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("schema: %s %s: %w", t.QualifiedName(), change, err)
		}
		changes = append(changes, Change{Type: t.QualifiedName(), Change: change, Statement: stmt})
		return nil
	}

	cols, err := q.TableColumns(ctx, table)
	if err != nil {
		return nil, fmt.Errorf("schema: read columns of %s: %w", table, err)
	}
	if len(cols) == 0 {
		defs := make([]string, 0, len(systemColumns))
		for _, c := range systemColumns {
			defs = append(defs, pgx.Identifier{c.name}.Sanitize()+" "+c.def)
		}
		stmt := fmt.Sprintf("CREATE TABLE %s (%s)", pgx.Identifier{table}.Sanitize(), strings.Join(defs, ", "))
		if err := run("create table", stmt); err != nil {
			return nil, err
		}
		for _, stmt := range systemIndexes(table) {
			if err := run("create index", stmt); err != nil {
				return nil, err
			}
		}
		cols = nil
	}
	have := map[string]string{}
	for _, c := range cols {
		have[c.Name] = c.UdtName
	}

	for _, f := range t.Fields {
		ct, err := ColumnType(f.Type)
		if err != nil {
			return nil, err
		}
		if udt, ok := have[f.Name]; ok {
			if udt != udtNames[ct] {
				// Changing a column type is a separate, job-driven process (architektur.md §3.2).
				return nil, fmt.Errorf("schema: %s field %s: column is %s, definition wants %s; changing a field type is not supported yet",
					t.QualifiedName(), f.Name, udt, ct)
			}
		} else {
			def := ct
			if f.Computed {
				// A sum kept by the core starts at zero, also for rows that exist (PL20).
				def += " NOT NULL DEFAULT 0"
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
				pgx.Identifier{table}.Sanitize(), pgx.Identifier{f.Name}.Sanitize(), def)
			if err := run("add column "+f.Name, stmt); err != nil {
				return nil, err
			}
		}
		if f.Unique {
			idx := table + "_" + f.Name + "_key"
			stmt := fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s) WHERE deleted_at IS NULL",
				pgx.Identifier{idx}.Sanitize(), pgx.Identifier{table}.Sanitize(), pgx.Identifier{f.Name}.Sanitize())
			exists, err := indexExists(ctx, tx, table, idx)
			if err != nil {
				return nil, err
			}
			if !exists {
				if err := run("unique index "+f.Name, stmt); err != nil {
					return nil, err
				}
			}
		}
	}
	// Full-text search (PL26, E101): GIN on the search column, trigram on the title field.
	title, others := t.SearchFields()
	if title != "" || len(others) > 0 {
		if err := ensureIndex(ctx, tx, table, table+"_search_idx",
			fmt.Sprintf("CREATE INDEX %s ON %s USING gin (search)", pgx.Identifier{table + "_search_idx"}.Sanitize(), pgx.Identifier{table}.Sanitize()), run); err != nil {
			return nil, err
		}
	}
	if title != "" {
		idx := table + "_" + title + "_trgm"
		if err := ensureIndex(ctx, tx, table, idx,
			fmt.Sprintf("CREATE INDEX %s ON %s USING gin (%s gin_trgm_ops)", pgx.Identifier{idx}.Sanitize(), pgx.Identifier{table}.Sanitize(), pgx.Identifier{title}.Sanitize()), run); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// ensureIndex creates an index unless it exists.
func ensureIndex(ctx context.Context, tx pgx.Tx, table, idx, stmt string, run func(change, stmt string) error) error {
	exists, err := indexExists(ctx, tx, table, idx)
	if err != nil || exists {
		return err
	}
	return run("index "+idx, stmt)
}

func systemIndexes(table string) []string {
	id := pgx.Identifier{table}.Sanitize()
	return []string{
		fmt.Sprintf("CREATE INDEX %s ON %s USING gist (org_path)", pgx.Identifier{table + "_org_path_idx"}.Sanitize(), id),
		fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (source, external_key) WHERE source IS NOT NULL", pgx.Identifier{table + "_source_key"}.Sanitize(), id),
		fmt.Sprintf("CREATE INDEX %s ON %s (updated_at)", pgx.Identifier{table + "_updated_at_idx"}.Sanitize(), id),
	}
}

// indexExists asks pg_indexes directly; sqlc does not know that catalog view.
func indexExists(ctx context.Context, tx pgx.Tx, table, idx string) (bool, error) {
	var n int
	err := tx.QueryRow(ctx,
		"SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2",
		table, idx).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("schema: read indexes of %s: %w", table, err)
	}
	return n > 0, nil
}
