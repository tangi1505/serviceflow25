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
	"os"
	"testing"
	"testing/fstest"
	"time"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// The acceptance of PL17: a type with the trait history keeps audit rows and answers its
// history, a type without does neither; events come for both.
func TestHistoryTraitDecidesOverAudit(t *testing.T) {
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
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_traitstest_kept", "DROP TABLE IF EXISTS rec_traitstest_plain",
		"DELETE FROM package WHERE name = 'traitstest'",
		"DELETE FROM audit WHERE type_name LIKE 'traitstest.%'",
		"DELETE FROM event WHERE type_name LIKE 'traitstest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'traitstest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":       {Data: []byte("name: traitstest\nversion: 0.1.0\nlabel: {de: T, en: T}\n")},
		"traits/tagged.yaml": {Data: []byte("name: tagged\nlabel: {de: G, en: T}\nfields:\n  - {name: tag_no, type: text, label: {de: N, en: N}}\n")},
		"types/kept.yaml":    {Data: []byte("name: kept\nlabel: {de: K, en: K}\ntraits: [history, tagged]\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
		"types/plain.yaml":   {Data: []byte("name: plain\nlabel: {de: P, en: P}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	s := New(db.Single(pool))
	traits, err := s.Traits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tr := range traits {
		if tr.Name == "tagged" && tr.Package == "traitstest" && len(tr.Fields) == 1 {
			found = true
		}
	}
	if !found || traits[0].Name != meta.TraitList {
		t.Fatalf("traits: %+v", traits)
	}

	count := func(table, typeName string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE type_name = $1", typeName).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, typeName := range []string{"traitstest.kept", "traitstest.plain"} {
		input := map[string]any{"title": "one"}
		if typeName == "traitstest.kept" {
			input["tag_no"] = "INV-1"
		}
		rec, err := s.Create(ctx, typeName, input, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Update(ctx, typeName, id(t, rec), 1, map[string]any{"title": "two"}, nil); err != nil {
			t.Fatal(err)
		}
		hist, err := s.History(ctx, typeName, id(t, rec))
		if err := s.Trash(ctx, typeName, id(t, rec), nil); err != nil {
			t.Fatal(err)
		}
		if n := count("event", typeName); n != 3 {
			t.Fatalf("%s: %d events", typeName, n)
		}
		switch typeName {
		case "traitstest.kept":
			// History before the trash: create, title, tag_no, update; the audit table has the trash row too.
			if err != nil || len(hist) != 4 || count("audit", typeName) != 5 {
				t.Fatalf("kept history: %d %v", len(hist), err)
			}
		case "traitstest.plain":
			if !errors.Is(err, ErrNotFound) || count("audit", typeName) != 0 {
				t.Fatalf("plain history: %v, %d rows", err, count("audit", typeName))
			}
		}
	}
}
