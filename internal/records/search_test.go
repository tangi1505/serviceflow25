// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const searchType = "searchtest.article"

func setupSearch(t *testing.T) (*Service, context.Context, *pgxpool.Pool) {
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
		"DROP TABLE IF EXISTS rec_searchtest_article",
		"DELETE FROM package WHERE name = 'searchtest'",
		"DELETE FROM translation WHERE type_name LIKE 'searchtest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'searchtest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml": {Data: []byte("name: searchtest\nversion: 0.1.0\nlabel: {de: S, en: S}\n")},
		"types/article.yaml": {Data: []byte(`name: article
label: {de: Artikel, en: Article}
title_field: title
fields:
  - {name: title, type: text, label: {de: T, en: T}, translatable: true}
  - {name: body, type: text, label: {de: B, en: B}, searchable: true}
  - {name: code, type: text, label: {de: C, en: C}}
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
	return New(db.Single(pool)), ctx, pool
}

func titles(hits []SearchHit) string {
	var out []string
	for _, h := range hits {
		if h.Title != nil {
			out = append(out, *h.Title)
		}
	}
	return strings.Join(out, "|")
}

// The acceptance of PL26: "Drucker" finds "Druckern" (German dictionary), "Drukcer" finds the
// title by trigram; the last word works as a prefix; English translations are found; the
// title weighs more than the body.
func TestSearch(t *testing.T) {
	s, ctx, pool := setupSearch(t)
	for _, in := range []map[string]any{
		{"title": "Störung an Druckern im 3. OG", "body": "Toner leer", "code": "XYZ123"},
		{"title": map[string]any{"de": "Drucker", "en": "Printer"}, "body": "Papierstau"},
		{"title": "Monitor flackert", "body": "seit dem Drucker-Update defekt"},
		{"title": "Tastatur", "body": "klemmt"},
	} {
		if _, err := s.Create(ctx, searchType, in, nil); err != nil {
			t.Fatal(err)
		}
	}
	find := func(q string) []SearchHit {
		t.Helper()
		hits, err := s.Search(ctx, SearchQuery{Text: q, Types: []string{searchType}})
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		return hits
	}
	hits := find("Drucker")
	if got := titles(hits); !strings.Contains(got, "Störung an Druckern im 3. OG") || !strings.Contains(got, "Monitor flackert") ||
		strings.Contains(got, "Tastatur") || !strings.HasPrefix(got, "Drucker|") && !strings.HasPrefix(got, "Störung") {
		t.Fatalf("Drucker: %s", got)
	}
	// The title weighs more than the body.
	if hits[len(hits)-1].Title == nil || *hits[len(hits)-1].Title != "Monitor flackert" {
		t.Fatalf("a body hit must come last: %s", titles(hits))
	}
	if !strings.Contains(hits[0].Snippet, "<mark>") {
		t.Fatalf("snippet: %q", hits[0].Snippet)
	}
	if got := titles(find("Drukcer")); !strings.Contains(got, "Drucker") {
		t.Fatalf("typo: %s", got)
	}
	if got := titles(find("Druck")); !strings.Contains(got, "Drucker") || !strings.Contains(got, "Druckern") {
		t.Fatalf("prefix: %s", got)
	}
	if got := titles(find("printer")); got != "Drucker" {
		t.Fatalf("English translation: %s", got)
	}
	if got := titles(find("Drucker -Toner")); strings.Contains(got, "Störung") {
		t.Fatalf("exclusion: %s", got)
	}
	if got := titles(find(`"Toner leer"`)); got != "Störung an Druckern im 3. OG" {
		t.Fatalf("phrase: %s", got)
	}
	// code is not searched.
	if got := titles(find("XYZ123")); got != "" {
		t.Fatalf("a field that is not searchable: %s", got)
	}
	if _, err := s.Search(ctx, SearchQuery{Text: strings.Repeat("x", 201)}); !isProblem(err, "q", "max_length") {
		t.Fatalf("too long: %v", err)
	}

	// Records from before PL26 have no search column; the job fills it.
	if _, err := pool.Exec(ctx, "UPDATE rec_searchtest_article SET search = NULL"); err != nil {
		t.Fatal(err)
	}
	if unfilled, err := s.SearchUnfilled(ctx, searchType); err != nil || !unfilled {
		t.Fatalf("unfilled: %v %v", unfilled, err)
	}
	if got := titles(find("Tastatur")); got != "Tastatur" {
		// Trigram on the title still finds it, the column is empty.
		t.Fatalf("title by trigram while the column is empty: %s", got)
	}
	if n, err := s.RecomputeSearch(ctx, searchType); err != nil || n != 4 {
		t.Fatalf("recompute: %d %v", n, err)
	}
	if got := titles(find("klemmt")); got != "Tastatur" {
		t.Fatalf("body after recompute: %s", got)
	}
	if unfilled, _ := s.SearchUnfilled(ctx, searchType); unfilled {
		t.Fatal("still unfilled")
	}
}

func TestPrefixParts(t *testing.T) {
	for in, want := range map[string][2]string{
		"Druck":          {"", "druck:*"},
		"Drucker im OG":  {"Drucker im", "og:*"},
		"Drucker -Toner": {"Drucker -Toner", ""},
		`"Toner leer"`:   {`"Toner leer"`, ""},
		"Drucker or":     {"Drucker or", ""},
		"a":              {"a", ""},
		"Drucker 3.":     {"Drucker 3.", ""},
		"Störung Etage":  {"Störung", "etage:*"},
	} {
		rest, last := prefixParts(in)
		if rest != want[0] || last != want[1] {
			t.Errorf("%q: %q %q, want %q %q", in, rest, last, want[0], want[1])
		}
	}
}
