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
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const articleType = "i18ntest.article"

// setupTranslations installs the package i18ntest: article with a translatable title and a
// plain code.
func setupTranslations(t *testing.T) (*Service, context.Context, *pgxpool.Pool) {
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
		"DROP TABLE IF EXISTS rec_i18ntest_article",
		"DELETE FROM package WHERE name = 'i18ntest'",
		"DELETE FROM audit WHERE type_name LIKE 'i18ntest.%'",
		"DELETE FROM translation WHERE type_name LIKE 'i18ntest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'i18ntest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml": {Data: []byte("name: i18ntest\nversion: 0.1.0\nlabel: {de: S, en: L}\n")},
		"types/article.yaml": {Data: []byte(`name: article
label: {de: Artikel, en: Article}
traits: [history]
fields:
  - {name: title, type: text, label: {de: T, en: T}, required: true, max_length: 20, translatable: true}
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

// read returns the record in a language (or every language).
func read(t *testing.T, s *Service, ctx context.Context, rec Record, l Lang) Record {
	t.Helper()
	got, err := s.Get(ctx, articleType, id(t, rec))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Localize(WithLang(ctx, l), articleType, got); err != nil {
		t.Fatal(err)
	}
	return got
}

// The acceptance of PL21: a record with a German and an English title returns the one of
// the requested language; without a translation it returns German.
func TestTranslatedValues(t *testing.T) {
	s, ctx, pool := setupTranslations(t)
	en := Lang{Read: "en", Write: "en"}
	rec, err := s.Create(ctx, articleType, map[string]any{"title": map[string]any{"de": "Drucker", "en": "Printer"}, "code": "P1"}, nil)
	if err != nil || rec["title"] != "Drucker" {
		t.Fatalf("create with two languages: %v %v", rec, err)
	}
	if got := read(t, s, ctx, rec, Lang{}); got["title"] != "Drucker" {
		t.Fatalf("German: %v", got["title"])
	}
	if got := read(t, s, ctx, rec, en); got["title"] != "Printer" || got["code"] != "P1" {
		t.Fatalf("English: %v", got)
	}
	all := read(t, s, ctx, rec, Lang{All: true})["title"].(map[string]any)
	if all["de"] != "Drucker" || all["en"] != "Printer" {
		t.Fatalf("all languages: %v", all)
	}
	plain, err := s.Create(ctx, articleType, map[string]any{"title": "Monitor"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, s, ctx, plain, en); got["title"] != "Monitor" {
		t.Fatalf("English falls back to German: %v", got["title"])
	}

	// A plain value in English goes to the translation; the German value stays.
	upd, err := s.Update(WithLang(ctx, en), articleType, id(t, rec), 1, map[string]any{"title": "Laser printer"}, nil)
	if err != nil || upd["version"] != int64(2) || upd["title"] != "Drucker" {
		t.Fatalf("update in English: %v %v", upd, err)
	}
	if got := read(t, s, ctx, rec, en); got["title"] != "Laser printer" {
		t.Fatalf("English after the update: %v", got["title"])
	}
	// The same value again is no change.
	if same, err := s.Update(WithLang(ctx, en), articleType, id(t, rec), 2, map[string]any{"title": "Laser printer"}, nil); err != nil || same["version"] != int64(2) {
		t.Fatalf("same translation again: %v %v", same, err)
	}
	// null removes the translation: English falls back to German.
	if _, err := s.Update(ctx, articleType, id(t, rec), 2, map[string]any{"title": map[string]any{"en": nil}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s, ctx, rec, en); got["title"] != "Drucker" {
		t.Fatalf("English after removing the translation: %v", got["title"])
	}

	// History: the translation is a field "title.en" of the version.
	versions, err := s.Versions(ctx, articleType, id(t, rec))
	if err != nil || len(versions) != 3 || strings.Join(versions[0].Changed, ",") != "title,code,title.en" ||
		strings.Join(versions[1].Changed, ",") != "title.en" {
		t.Fatalf("versions: %+v %v", versions, err)
	}

	// Purge removes the translations.
	if err := s.Trash(ctx, articleType, id(t, plain), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(WithLang(ctx, en), articleType, id(t, rec), 3, map[string]any{"title": "Printer"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, articleType, id(t, rec), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, articleType, id(t, rec), nil); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM translation WHERE record_id = $1", id(t, rec)).Scan(&left); err != nil || left != 0 {
		t.Fatalf("translations after purge: %d %v", left, err)
	}
}

// Wrong languages and values are refused field by field.
func TestTranslationProblems(t *testing.T) {
	s, ctx, _ := setupTranslations(t)
	cases := []struct {
		ctx   context.Context
		input map[string]any
		field string
		code  string
	}{
		{ctx, map[string]any{"title": map[string]any{"de": "A", "fr": "B"}}, "title", "language"},
		{WithLang(ctx, Lang{Write: "fr"}), map[string]any{"title": "A"}, "title", "language"},
		{ctx, map[string]any{"title": map[string]any{"de": "A", "en": strings.Repeat("x", 21)}}, "title.en", "max_length"},
		{ctx, map[string]any{"title": map[string]any{"de": "A", "en": 5.0}}, "title.en", "type"},
		{WithLang(ctx, Lang{Write: "en"}), map[string]any{"title": "Only English"}, "title", "required"},
		{ctx, map[string]any{"title": "A", "code": map[string]any{"de": "x"}}, "code", "type"},
	}
	for _, c := range cases {
		_, err := s.Create(c.ctx, articleType, c.input, nil)
		if !isProblem(err, c.field, c.code) {
			t.Errorf("%v: want %s/%s, got %v", c.input, c.field, c.code, err)
		}
	}
	// A plain value written in German, or without a language, stays in the column.
	rec, err := s.Create(WithLang(ctx, Lang{Write: "de"}), articleType, map[string]any{"title": "Tastatur", "code": "K1"}, nil)
	if err != nil || rec["title"] != "Tastatur" {
		t.Fatalf("German write: %v %v", rec, err)
	}
	if _, err := s.Update(ctx, articleType, id(t, rec), 7, map[string]any{"title": map[string]any{"en": "Keyboard"}}, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("a translation keeps the version check: %v", err)
	}
}
