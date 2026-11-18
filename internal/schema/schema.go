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
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	// The tenant of the node, filled by the trigger set_tenant (PL46): unique values and
	// external keys are unique per tenant.
	{"tenant", "ltree NOT NULL DEFAULT 'root'"},
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
	// The next deadline of the running SLA clocks, their three-quarter point and whether only
	// paused clocks are left (UI34, migration 41): kept by internal/sla, not fields.
	{"sla_due_at", "timestamptz"},
	{"sla_warn_at", "timestamptz"},
	{"sla_paused", "boolean NOT NULL DEFAULT false"},
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
	case meta.FieldSequence:
		return "bigint", nil
	}
	return "", fmt.Errorf("schema: no column type for field type %q", ft)
}

// udtNames is what information_schema reports for each column type.
var udtNames = map[string]string{
	"text": "text", "numeric": "numeric", "boolean": "bool", "date": "date", "timestamptz": "timestamptz", "uuid": "uuid",
	"bigint": "int8",
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
	// The tenant column and its trigger (PL46); a table from before gets them here, and its
	// unique indexes are rebuilt per tenant below.
	if _, ok := have["tenant"]; !ok && len(cols) > 0 {
		id := pgx.Identifier{table}.Sanitize()
		if err := run("add column tenant", fmt.Sprintf("ALTER TABLE %s ADD COLUMN tenant ltree NOT NULL DEFAULT 'root'", id)); err != nil {
			return nil, err
		}
		if err := run("fill tenant", fmt.Sprintf("UPDATE %s SET tenant = tenant_of(org_path)", id)); err != nil {
			return nil, err
		}
	}
	if err := ensureTrigger(ctx, tx, table, run); err != nil {
		return nil, err
	}
	if err := ensureVacuum(ctx, tx, table, run); err != nil {
		return nil, err
	}
	if err := ensureUnique(ctx, tx, table, table+"_source_key", sourceKeyIndex(table), run); err != nil {
		return nil, err
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
			if f.StartsAtZero() {
				// A sum kept by the core starts at zero, also for rows that exist (PL20, WF13).
				// Other computed fields start empty, the outcome of an approval for one (WF6), the
				// durations of a resolution (UI14a).
				def += " NOT NULL DEFAULT 0"
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
				pgx.Identifier{table}.Sanitize(), pgx.Identifier{f.Name}.Sanitize(), def)
			if err := run("add column "+f.Name, stmt); err != nil {
				return nil, err
			}
		}
		if f.Type == meta.FieldSequence {
			// A running number is unique per tenant, the trash included: it is never given
			// twice (PL49).
			idx := table + "_" + f.Name + "_key"
			stmt := fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (tenant, %s)",
				pgx.Identifier{idx}.Sanitize(), pgx.Identifier{table}.Sanitize(), pgx.Identifier{f.Name}.Sanitize())
			if err := ensureUnique(ctx, tx, table, idx, stmt, run); err != nil {
				return nil, err
			}
		} else if f.Unique {
			// Unique per tenant (PL46, E136), outside the trash.
			idx := table + "_" + f.Name + "_key"
			stmt := fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (tenant, %s) WHERE deleted_at IS NULL",
				pgx.Identifier{idx}.Sanitize(), pgx.Identifier{table}.Sanitize(), pgx.Identifier{f.Name}.Sanitize())
			if err := ensureUnique(ctx, tx, table, idx, stmt, run); err != nil {
				return nil, err
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
	// The moment of creation of every type (PL54): lists sort by it, newest first, and a
	// table of millions without it is read whole (found with H14).
	if err := ensureIndex(ctx, tx, table, table+"_created_at_idx",
		fmt.Sprintf("CREATE INDEX %s ON %s (created_at)", pgx.Identifier{table + "_created_at_idx"}.Sanitize(), pgx.Identifier{table}.Sanitize()), run); err != nil {
		return nil, err
	}
	// The moment of the resolution of a type with the trait resolution (PL66, Hauptplan E195):
	// the numbers of "Statistik" count what was resolved in a period. Only resolved records
	// are in it, the open ones of a queue are not.
	if f, ok := t.Field(meta.FieldResolvedAt); ok && f.Type == meta.FieldDateTime {
		idx := table + "_resolved_at_idx"
		if err := ensureIndex(ctx, tx, table, idx,
			fmt.Sprintf("CREATE INDEX %s ON %s (%s) WHERE %s IS NOT NULL", pgx.Identifier{idx}.Sanitize(), pgx.Identifier{table}.Sanitize(),
				pgx.Identifier{meta.FieldResolvedAt}.Sanitize(), pgx.Identifier{meta.FieldResolvedAt}.Sanitize()), run); err != nil {
			return nil, err
		}
	}
	if err := ensureDeclared(ctx, tx, t, table, run); err != nil {
		return nil, err
	}
	return changes, nil
}

// declaredMark is the comment on the indexes a package declared (PL54): those, and only those,
// go again when the definition no longer names them.
const declaredMark = "itsm: declared by the type (PL54)"

// DeclaredIndexName is the name of a declared index: the table, "ix" ("ux" when unique) and
// the columns; a name longer than PostgreSQL keeps ends in a hash instead.
func DeclaredIndexName(table string, ix meta.Index) string {
	kind := "_ix_"
	if ix.Unique {
		kind = "_ux_"
	}
	name := table + kind + strings.Join(ix.Fields, "_")
	if len(name) <= 63 {
		return name
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return fmt.Sprintf("%s_%08x", name[:54], h.Sum32())
}

// ensureDeclared creates the indexes the type declares and drops the declared ones it no
// longer names (PL54). The build of an index on a large table holds its writes for as long as
// it takes, inside the transaction of the install.
func ensureDeclared(ctx context.Context, tx pgx.Tx, t meta.Type, table string, run func(change, stmt string) error) error {
	want := map[string]bool{}
	for _, ix := range t.Indexes {
		name := DeclaredIndexName(table, ix)
		want[name] = true
		exists, err := indexExists(ctx, tx, table, name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		cols := make([]string, len(ix.Fields))
		for i, f := range ix.Fields {
			cols[i] = pgx.Identifier{f}.Sanitize()
		}
		stmt := fmt.Sprintf("CREATE INDEX %s ON %s (%s)",
			pgx.Identifier{name}.Sanitize(), pgx.Identifier{table}.Sanitize(), strings.Join(cols, ", "))
		if ix.Unique {
			// Unique per tenant and outside the trash, like a unique field (PL46).
			stmt = fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (tenant, %s) WHERE deleted_at IS NULL",
				pgx.Identifier{name}.Sanitize(), pgx.Identifier{table}.Sanitize(), strings.Join(cols, ", "))
		}
		if err := run("index "+name, stmt); err != nil {
			var pgErr *pgconn.PgError
			if ix.Unique && errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("schema: %s: %s must be unique, but the table holds doubles; remove them first: %w",
					t.QualifiedName(), strings.Join(ix.Fields, ", "), err)
			}
			return err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf("COMMENT ON INDEX %s IS '%s'", pgx.Identifier{name}.Sanitize(), declaredMark)); err != nil {
			return fmt.Errorf("schema: mark index %s: %w", name, err)
		}
	}
	rows, err := tx.Query(ctx, `SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
WHERE i.indrelid = $1::regclass AND obj_description(c.oid, 'pg_class') = $2 ORDER BY 1`, pgx.Identifier{table}.Sanitize(), declaredMark)
	if err != nil {
		return fmt.Errorf("schema: read declared indexes of %s: %w", table, err)
	}
	have, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("schema: read declared indexes of %s: %w", table, err)
	}
	for _, name := range have {
		if want[name] {
			continue
		}
		if err := run("drop index "+name, "DROP INDEX "+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
	}
	return nil
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
		sourceKeyIndex(table),
		fmt.Sprintf("CREATE INDEX %s ON %s (updated_at)", pgx.Identifier{table + "_updated_at_idx"}.Sanitize(), id),
		fmt.Sprintf("CREATE INDEX %s ON %s (sla_due_at, id)", pgx.Identifier{table + "_sla_due_idx"}.Sanitize(), id),
	}
}

// sourceKeyIndex makes the external key of a source unique per tenant (PL46).
func sourceKeyIndex(table string) string {
	return fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (tenant, source, external_key) WHERE source IS NOT NULL",
		pgx.Identifier{table + "_source_key"}.Sanitize(), pgx.Identifier{table}.Sanitize())
}

// ensureUnique creates a unique index, or rebuilds one from before the tenant column (PL46):
// the definition of a current one names tenant first.
func ensureUnique(ctx context.Context, tx pgx.Tx, table, idx, stmt string, run func(change, stmt string) error) error {
	def, err := indexDef(ctx, tx, table, idx)
	if err != nil {
		return err
	}
	if strings.Contains(def, "(tenant, ") {
		return nil
	}
	if def != "" {
		if err := run("drop index "+idx, "DROP INDEX "+pgx.Identifier{idx}.Sanitize()); err != nil {
			return err
		}
	}
	return run("unique index "+idx, stmt)
}

// ensureTrigger puts the trigger that fills the tenant column on the table (PL46).
func ensureTrigger(ctx context.Context, tx pgx.Tx, table string, run func(change, stmt string) error) error {
	name := table + "_tenant"
	var n int
	err := tx.QueryRow(ctx, "SELECT count(*) FROM pg_trigger WHERE tgname = $1 AND tgrelid = $2::regclass", name, pgx.Identifier{table}.Sanitize()).Scan(&n)
	if err != nil {
		return fmt.Errorf("schema: read triggers of %s: %w", table, err)
	}
	if n > 0 {
		return nil
	}
	return run("trigger tenant", fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT OR UPDATE OF org_path ON %s FOR EACH ROW EXECUTE FUNCTION set_tenant()",
		pgx.Identifier{name}.Sanitize(), pgx.Identifier{table}.Sanitize()))
}

// vacuumOptions are the thresholds of autovacuum for a type table (PL73, Kern-Plan E77). By
// default PostgreSQL cleans a table when a fifth of its rows are dead and analyses it when a
// tenth changed: with 18 million incidents 3.6 million dead rows, which every change of an
// incident and every cleanup of a measurement leave, mostly among the newest. With 0.2 % and
// 1 % a table of 18 million rows is cleaned from about 37,000 dead rows and analysed from about
// 180,000 changes; a small one from about 1,000, as the threshold of 1,000 adds.
var vacuumOptions = []string{
	"autovacuum_vacuum_scale_factor=0.002",
	"autovacuum_vacuum_threshold=1000",
	"autovacuum_analyze_scale_factor=0.01",
	"autovacuum_analyze_threshold=1000",
}

// ensureVacuum sets vacuumOptions on the table when one of them is missing or different.
func ensureVacuum(ctx context.Context, tx pgx.Tx, table string, run func(change, stmt string) error) error {
	var have []string
	err := tx.QueryRow(ctx, "SELECT coalesce(reloptions, '{}') FROM pg_class WHERE oid = $1::regclass", pgx.Identifier{table}.Sanitize()).Scan(&have)
	if err != nil {
		return fmt.Errorf("schema: read options of %s: %w", table, err)
	}
	for _, o := range vacuumOptions {
		if !slices.Contains(have, o) {
			return run("autovacuum", fmt.Sprintf("ALTER TABLE %s SET (%s)", pgx.Identifier{table}.Sanitize(), strings.Join(vacuumOptions, ", ")))
		}
	}
	return nil
}

// indexDef is the definition of an index of the table, empty when there is none.
func indexDef(ctx context.Context, tx pgx.Tx, table, idx string) (string, error) {
	var def string
	err := tx.QueryRow(ctx,
		"SELECT coalesce(max(indexdef), '') FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2",
		table, idx).Scan(&def)
	if err != nil {
		return "", fmt.Errorf("schema: read index %s of %s: %w", idx, table, err)
	}
	return def, nil
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
