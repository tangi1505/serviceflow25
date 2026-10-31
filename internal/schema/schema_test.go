// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package schema

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func testPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func lbl(s string) meta.Label { return meta.Label{"de": s, "en": s} }

func typ(name string, fields ...meta.Field) meta.Type {
	return meta.Type{Package: "schematest", Name: name, Label: lbl(name), Fields: fields}
}

// drop removes the table and its schema log, so that the test starts clean in a persistent database.
func drop(t *testing.T, pool *pgxpool.Pool, ctx context.Context, table string) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
		t.Fatal(err)
	}
	typeName := "schematest." + strings.TrimPrefix(table, "rec_schematest_")
	if _, err := pool.Exec(ctx, "DELETE FROM schema_change WHERE type_name = $1", typeName); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTableAddColumnUniqueIndexAndLog(t *testing.T) {
	pool, ctx := testPool(t)
	drop(t, pool, ctx, "rec_schematest_alpha")
	q := queries.New(pool)

	ty := typ("alpha",
		meta.Field{Name: "title", Type: meta.FieldText, Label: lbl("t"), Unique: true},
		meta.Field{Name: "priority", Type: meta.FieldNumber, Label: lbl("p")},
		meta.Field{Name: "active", Type: meta.FieldBoolean, Label: lbl("a")},
		meta.Field{Name: "due_on", Type: meta.FieldDate, Label: lbl("d")},
		meta.Field{Name: "seen_at", Type: meta.FieldDateTime, Label: lbl("s")},
		meta.Field{Name: "kind", Type: meta.FieldChoice, Label: lbl("k")},
		meta.Field{Name: "parent", Type: meta.FieldReference, Label: lbl("r"), To: "schematest.alpha"},
	)
	changes, err := Apply(ctx, pool, []meta.Type{ty}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	// create table + 3 system indexes + the tenant trigger (PL46) + 7 columns + 1 unique index
	if len(changes) != 13 {
		t.Fatalf("got %d changes, want 13: %+v", len(changes), changes)
	}

	cols, err := q.TableColumns(ctx, "rec_schematest_alpha")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cols {
		got[c.Name] = c.UdtName
	}
	want := map[string]string{
		"id": "uuid", "org_path": "ltree", "tenant": "ltree", "version": "int8", "state": "text", "source": "text",
		"external_key": "text", "row_hash": "bytea", "search": "tsvector", "created_at": "timestamptz",
		"created_by": "uuid", "updated_at": "timestamptz", "updated_by": "uuid", "deleted_at": "timestamptz",
		"title": "text", "priority": "numeric", "active": "bool", "due_on": "date", "seen_at": "timestamptz",
		"kind": "text", "parent": "uuid",
	}
	for name, udt := range want {
		if got[name] != udt {
			t.Errorf("column %s: got %q, want %q", name, got[name], udt)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d columns, want %d: %v", len(got), len(want), got)
	}

	rows, err := pool.Query(ctx, "SELECT indexname FROM pg_indexes WHERE tablename = 'rec_schematest_alpha'")
	if err != nil {
		t.Fatal(err)
	}
	var idx []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		idx = append(idx, n)
	}
	rows.Close()
	all := strings.Join(idx, ",")
	for _, w := range []string{"rec_schematest_alpha_title_key", "rec_schematest_alpha_org_path_idx", "rec_schematest_alpha_source_key"} {
		if !strings.Contains(all, w) {
			t.Errorf("index %s missing in %s", w, all)
		}
	}

	// Unique index excludes deleted rows and works.
	if _, err := pool.Exec(ctx, "INSERT INTO rec_schematest_alpha (id, title) VALUES (gen_random_uuid(), 'x'), (gen_random_uuid(), 'x')"); err == nil {
		t.Error("duplicate title must be rejected")
	}

	// Second apply: nothing to do.
	changes, err = Apply(ctx, pool, []meta.Type{ty}, quiet)
	if err != nil || len(changes) != 0 {
		t.Fatalf("second apply: %v, %d changes", err, len(changes))
	}

	// New field: one column added, table not recreated.
	ty.Fields = append(ty.Fields, meta.Field{Name: "note", Type: meta.FieldText, Label: lbl("n")})
	changes, err = Apply(ctx, pool, []meta.Type{ty}, quiet)
	if err != nil || len(changes) != 1 || changes[0].Change != "add column note" {
		t.Fatalf("add column: %v, %+v", err, changes)
	}

	// Type change is refused with the field name.
	ty.Fields[1].Type = meta.FieldText
	if _, err := Apply(ctx, pool, []meta.Type{ty}, quiet); err == nil || !strings.Contains(err.Error(), "priority") {
		t.Fatalf("type change must be refused naming the field, got %v", err)
	}

	log, err := q.ListSchemaChanges(ctx, "schematest.alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(log) < 13 {
		t.Fatalf("schema log has %d rows, want at least 13", len(log))
	}
}

func TestParallelApplyEndsWithOneTable(t *testing.T) {
	pool, ctx := testPool(t)
	drop(t, pool, ctx, "rec_schematest_beta")
	ty := typ("beta", meta.Field{Name: "title", Type: meta.FieldText, Label: lbl("t"), Unique: true})

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = Apply(ctx, pool, []meta.Type{ty}, quiet)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("apply %d: %v", i, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name = 'rec_schematest_beta'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d tables", n)
	}
	q := queries.New(pool)
	log, err := q.ListSchemaChanges(ctx, "schematest.beta")
	if err != nil {
		t.Fatal(err)
	}
	// Exactly one creation: create table + 3 indexes + the tenant trigger (PL46) + 1 column
	// + 1 unique index.
	if len(log) != 7 {
		t.Fatalf("schema log has %d rows, want 7 (one creation only)", len(log))
	}
}

func TestColumnTypeCoversAllFieldTypes(t *testing.T) {
	for _, ft := range meta.FieldTypes {
		if _, err := ColumnType(ft); err != nil {
			t.Error(err)
		}
	}
	if _, err := ColumnType("colour"); err == nil {
		t.Error("unknown type must fail")
	}
}
