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
	"errors"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const (
	tagType    = "liststest.tag"
	regionType = "liststest.region"
	itemType   = "liststest.item"
)

// setupLists installs a package with a flat list, a tree list with coordinates and a type
// that references both, and seeds the shipped entries.
func setupLists(t *testing.T) (*Service, context.Context, *pgxpool.Pool, meta.Package) {
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
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_liststest_item",
		"DELETE FROM list_entry WHERE list LIKE 'liststest.%'",
		"DELETE FROM package WHERE name = 'liststest'",
		"DELETE FROM audit WHERE type_name LIKE 'liststest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'liststest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":      {Data: []byte("name: liststest\nversion: 0.1.0\nlabel: {de: L, en: L}\n")},
		"types/tag.yaml":    {Data: []byte("name: tag\nlabel: {de: Tag, en: Tag}\ntraits: [list]\nlist:\n  entries:\n    - {key: a, name: A, position: 1}\n    - {key: b, name: B, position: 2}\n")},
		"types/region.yaml": {Data: []byte("name: region\nlabel: {de: Region, en: Region}\ntraits: [list]\nlist:\n  tree: true\n  coordinates: true\n  entries:\n    - {key: eu, name: Europa}\n    - {key: de, name: Deutschland, parent: eu, lat: 51, lon: 10}\n    - {key: nrw, name: NRW, parent: de}\n    - {key: us, name: USA}\n")},
		"types/item.yaml": {Data: []byte(`name: item
label: {de: Ding, en: Item}
fields:
  - {name: title, type: text, label: {de: T, en: T}, required: true}
  - {name: tag, type: reference, label: {de: Tag, en: Tag}, to: liststest.tag}
  - {name: region, type: reference, label: {de: R, en: R}, to: liststest.region}
`)},
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
	s := New(pool)
	n, err := s.SeedLists(ctx, p)
	if err != nil || n != 6 {
		t.Fatalf("seed: %d %v", n, err)
	}
	return s, ctx, pool, p
}

func byKey(t *testing.T, s *Service, ctx context.Context, typeName, key string) Record {
	t.Helper()
	page, err := s.List(ctx, typeName, ListOptions{Filter: map[string]string{"key": key}})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("entry %s of %s: %v %v", key, typeName, page.Items, err)
	}
	return page.Items[0]
}

// The acceptance of PL16: two lists, one a tree, no table per list; a reference refuses a
// value outside its list; seeds are idempotent; the tree keeps path and full name.
func TestListsShareOneTableAndRefuseForeignValues(t *testing.T) {
	s, ctx, pool, p := setupLists(t)
	for _, table := range []string{"rec_liststest_tag", "rec_liststest_region"} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name = $1", table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("table %s exists: %d %v", table, n, err)
		}
	}
	if n, err := s.SeedLists(ctx, p); err != nil || n != 0 {
		t.Fatalf("second seed: %d %v", n, err)
	}
	a := byKey(t, s, ctx, tagType, "a")
	eu := byKey(t, s, ctx, regionType, "eu")
	if a["name"] != "A" || a["active"] != true || a["position"] != 1.0 || a[fullNameColumn] != nil {
		t.Fatalf("tag a: %v", a)
	}

	// A reference to a list takes entries of that list only: an entry of region is no tag.
	_, err := s.Create(ctx, itemType, map[string]any{"title": "x", "tag": eu["id"]}, nil)
	ve, ok := AsValidationError(err)
	if !ok || len(ve.Problems) != 1 || ve.Problems[0].Field != "tag" || ve.Problems[0].Code != "reference" {
		t.Fatalf("foreign value: %v", err)
	}
	item, err := s.Create(ctx, itemType, map[string]any{"title": "x", "tag": a["id"], "region": eu["id"]}, nil)
	if err != nil || item["tag"] != a["id"] {
		t.Fatalf("item: %v %v", item, err)
	}
	// Reading an entry through the other list is not found (the scope of every statement).
	if _, err := s.Get(ctx, tagType, id(t, eu)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tag by region id: %v", err)
	}
	// Keys are unique per list, not across lists.
	if _, err := s.Create(ctx, tagType, map[string]any{"name": "A2", "key": "a"}, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate key: %v", err)
	}
	if _, err := s.Create(ctx, regionType, map[string]any{"name": "A region", "key": "a"}, nil); err != nil {
		t.Fatalf("same key in another list: %v", err)
	}
	// Lists answer in name order when asked; the list of tags has exactly its own entries.
	page, err := s.List(ctx, tagType, ListOptions{Count: true})
	if err != nil || *page.Total != 2 {
		t.Fatalf("tags: %v %v", page, err)
	}
}

func TestTreeListKeepsPathAndFullName(t *testing.T) {
	s, ctx, _, _ := setupLists(t)
	eu, de, nrw, us := byKey(t, s, ctx, regionType, "eu"), byKey(t, s, ctx, regionType, "de"), byKey(t, s, ctx, regionType, "nrw"), byKey(t, s, ctx, regionType, "us")
	if nrw[fullNameColumn] != "Europa > Deutschland > NRW" || de["lat"] != 51.0 || us[fullNameColumn] != "USA" {
		t.Fatalf("full names: %v %v", nrw, us)
	}
	// Below eu: de and nrw, not eu itself and not us.
	page, err := s.List(ctx, regionType, ListOptions{Filter: map[string]string{"under": eu["id"].(string)}})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("under eu: %v %v", page.Items, err)
	}
	under, _ := json.Marshal(map[string]any{"field": "parent", "under": eu["id"]})
	qp, _, err := s.Query(ctx, regionType, Query{Where: under, Sort: []string{"name"}, Fields: []string{"name", fullNameColumn}})
	if err != nil || len(qp.Items) != 2 || qp.Items[0]["name"] != "Deutschland" || qp.Items[1][fullNameColumn] != "Europa > Deutschland > NRW" {
		t.Fatalf("query under: %v %v", qp.Items, err)
	}
	// A cycle is refused: eu below nrw.
	_, err = s.Update(ctx, regionType, id(t, eu), eu["version"].(int64), map[string]any{"parent": nrw["id"]}, nil)
	if ve, ok := AsValidationError(err); !ok || ve.Problems[0].Code != "cycle" {
		t.Fatalf("cycle: %v", err)
	}
	// Moving de under us renames the whole subtree; renaming us does too.
	if _, err := s.Update(ctx, regionType, id(t, de), de["version"].(int64), map[string]any{"parent": us["id"]}, nil); err != nil {
		t.Fatal(err)
	}
	us = byKey(t, s, ctx, regionType, "us")
	if _, err := s.Update(ctx, regionType, id(t, us), us["version"].(int64), map[string]any{"name": "Amerika"}, nil); err != nil {
		t.Fatal(err)
	}
	if nrw = byKey(t, s, ctx, regionType, "nrw"); nrw[fullNameColumn] != "Amerika > Deutschland > NRW" {
		t.Fatalf("after the move: %v", nrw[fullNameColumn])
	}
	page, err = s.List(ctx, regionType, ListOptions{Filter: map[string]string{"under": eu["id"].(string)}})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("under eu after the move: %v %v", page.Items, err)
	}
	// A path through the reference reaches the entry's fields.
	where, _ := json.Marshal(map[string]any{"field": "region.name", "eq": "Europa"})
	item, err := s.Create(ctx, itemType, map[string]any{"title": "in eu", "region": eu["id"]}, nil)
	if err != nil {
		t.Fatal(err)
	}
	qp, _, err = s.Query(ctx, itemType, Query{Where: where, Fields: []string{"title", "region.name"}})
	if err != nil || len(qp.Items) != 1 || qp.Items[0]["id"] != item["id"] || qp.Items[0]["region.name"] != "Europa" {
		t.Fatalf("path into a list: %v %v", qp.Items, err)
	}
}
