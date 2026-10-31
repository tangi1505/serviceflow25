// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- translated field values (KF19; Hauptplan E98; PL21) ---
//
// A text field with translatable: true keeps a value per language. Its column holds the
// German value, the fallback of every language; the other languages live in the core table
// translation. Reading puts the value of the requested language in place of the German one
// where there is one (Localize); all languages at once come as an object {de, en}. Writing a
// plain value goes to the language of the write (Content-Language, German by default); an
// object sets several languages at once, null removes a translation. A translation is a
// change of the record like any other: new version, audit row with the field "title.en",
// event.

// Lang is the language of a request: Read for the values returned, Write for plain values
// written to translatable fields, All for every language as an object.
type Lang struct {
	Read  string
	Write string
	All   bool
}

type langKey struct{}

// WithLang sets the language of the request on the context (the HTTP layer reads
// Accept-Language, Content-Language and ?translations=all).
func WithLang(ctx context.Context, l Lang) context.Context {
	return context.WithValue(ctx, langKey{}, l)
}

// LangOf returns the language of the request; German without one.
func LangOf(ctx context.Context) Lang {
	l, _ := ctx.Value(langKey{}).(Lang)
	if l.Read == "" {
		l.Read = meta.DefaultLanguage
	}
	if l.Write == "" {
		l.Write = meta.DefaultLanguage
	}
	return l
}

// translations are new values of other languages: field → language → value (nil removes).
type translations map[string]map[string]any

// splitTranslations takes the values of other languages out of the input: what stays is the
// input with the German values, checked as usual by validate.
func splitTranslations(ctx context.Context, t meta.Type, input map[string]any) (map[string]any, translations, []Problem) {
	write := LangOf(ctx).Write
	var tr translations
	var ps []Problem
	out := input
	copied := false
	own := func() {
		if !copied {
			out = make(map[string]any, len(input))
			for k, v := range input {
				out[k] = v
			}
			copied = true
		}
	}
	set := func(f meta.Field, lang string, v any) {
		if !meta.IsLanguage(lang) {
			ps = append(ps, Problem{Field: f.Name, Code: "language", Message: fmt.Sprintf("%q is no language of the installation, use %s", lang, strings.Join(meta.Languages, " or "))})
			return
		}
		if lang == meta.DefaultLanguage {
			own()
			out[f.Name] = v
			return
		}
		cv, p := coerce(f, v)
		if p != nil {
			p.Field = f.Name + "." + lang
			ps = append(ps, *p)
			return
		}
		if s, ok := cv.(string); ok && s == "" {
			cv = nil // an empty translation is no translation
		}
		if tr == nil {
			tr = translations{}
		}
		if tr[f.Name] == nil {
			tr[f.Name] = map[string]any{}
		}
		tr[f.Name][lang] = cv
	}
	for _, f := range t.Fields {
		v, given := input[f.Name]
		if !given || !f.Translatable {
			continue
		}
		if byLang, ok := v.(map[string]any); ok {
			own()
			delete(out, f.Name)
			for lang, lv := range byLang {
				set(f, lang, lv)
			}
			continue
		}
		if write != meta.DefaultLanguage {
			own()
			delete(out, f.Name)
			set(f, write, v)
		}
	}
	return out, tr, ps
}

// loadTranslations reads the translations of records: id → field → language → value.
func loadTranslations(ctx context.Context, q *queries.Queries, typeName string, ids []uuid.UUID) (map[uuid.UUID]map[string]map[string]string, error) {
	rows, err := q.TranslationsOf(ctx, queries.TranslationsOfParams{TypeName: typeName, RecordIds: ids})
	if err != nil {
		return nil, fmt.Errorf("records: translations: %w", err)
	}
	out := map[uuid.UUID]map[string]map[string]string{}
	for _, r := range rows {
		if out[r.RecordID] == nil {
			out[r.RecordID] = map[string]map[string]string{}
		}
		if out[r.RecordID][r.Field] == nil {
			out[r.RecordID][r.Field] = map[string]string{}
		}
		out[r.RecordID][r.Field][r.Language] = r.Value
	}
	return out, nil
}

// translationChanges compares the new translations with the stored ones; only differences
// are changes.
func translationChanges(tr translations, old map[string]map[string]string) []change {
	var out []change
	for _, f := range sortedKeys(tr) {
		for _, lang := range sortedKeys(tr[f]) {
			var before any
			if v, ok := old[f][lang]; ok {
				before = v
			}
			after := tr[f][lang]
			if equalJSON(before, after) {
				continue
			}
			out = append(out, change{f + "." + lang, before, after})
		}
	}
	return out
}

// writeTranslations stores the changes of translations (field "title.en").
func writeTranslations(ctx context.Context, q *queries.Queries, typeName string, id uuid.UUID, changes []change) error {
	for _, c := range changes {
		f, lang, ok := strings.Cut(c.field, ".")
		if !ok {
			continue
		}
		var err error
		if c.new == nil {
			err = q.DeleteTranslation(ctx, queries.DeleteTranslationParams{TypeName: typeName, RecordID: id, Field: f, Language: lang})
		} else {
			err = q.UpsertTranslation(ctx, queries.UpsertTranslationParams{TypeName: typeName, RecordID: id, Field: f, Language: lang, Value: c.new.(string)})
		}
		if err != nil {
			return fmt.Errorf("records: write translation %s: %w", c.field, err)
		}
	}
	return nil
}

// isTranslation reports whether a change is one of a translation.
func isTranslation(c change) bool { return strings.Contains(c.field, ".") && !isActorChange(c) }

// Localize puts the values of the request's language into records of the type read before
// (Get, List, Query, writes). Fields the session may not read are already gone and stay gone.
// It returns the language of the values.
func (s *Service) Localize(ctx context.Context, typeName string, recs ...Record) (string, error) {
	l := LangOf(ctx)
	if (l.Read == meta.DefaultLanguage && !l.All) || len(recs) == 0 {
		// The columns hold German: nothing to load (LZ1, LZ2 read this way).
		return l.Read, nil
	}
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return "", err
	}
	var fields []string
	for _, f := range t.Fields {
		if f.Translatable {
			fields = append(fields, f.Name)
		}
	}
	if len(fields) == 0 {
		return l.Read, nil
	}
	ids := make([]uuid.UUID, 0, len(recs))
	for _, rec := range recs {
		if id, err := uuid.Parse(fmt.Sprint(rec["id"])); err == nil {
			ids = append(ids, id)
		}
	}
	all, err := loadTranslations(ctx, queries.New(s.db.Tenant(ctx)), typeName, ids)
	if err != nil {
		return "", err
	}
	for _, rec := range recs {
		id, _ := uuid.Parse(fmt.Sprint(rec["id"]))
		for _, f := range fields {
			base, shown := rec[f]
			if !shown {
				continue
			}
			if l.All {
				byLang := map[string]any{meta.DefaultLanguage: base}
				for _, lang := range meta.Languages {
					if lang == meta.DefaultLanguage {
						continue
					}
					byLang[lang] = nil
					if v, ok := all[id][f][lang]; ok {
						byLang[lang] = v
					}
				}
				rec[f] = byLang
				continue
			}
			if v, ok := all[id][f][l.Read]; ok {
				rec[f] = v
			}
		}
	}
	return l.Read, nil
}

// withProblems adds the problems of the translations to the result of validate.
func withProblems(err error, ps []Problem) error {
	if len(ps) == 0 {
		return err
	}
	if err == nil {
		return &ValidationError{Problems: ps}
	}
	if ve, ok := AsValidationError(err); ok {
		ve.Problems = append(ve.Problems, ps...)
		return ve
	}
	return err
}

// withTranslated adds the translated fields to the values whose write rights are checked:
// translating a field needs the right to write it.
func withTranslated(values map[string]any, tr translations) map[string]any {
	if len(tr) == 0 {
		return values
	}
	out := make(map[string]any, len(values)+len(tr))
	for k, v := range values {
		out[k] = v
	}
	for f := range tr {
		if _, ok := out[f]; !ok {
			out[f] = nil
		}
	}
	return out
}

// purgeTranslations removes the translations of a record that is purged.
func purgeTranslations(ctx context.Context, tx pgx.Tx, typeName string, id uuid.UUID) error {
	if err := queries.New(tx).DeleteTranslationsOfRecord(ctx, queries.DeleteTranslationsOfRecordParams{TypeName: typeName, RecordID: id}); err != nil {
		return fmt.Errorf("records: purge translations: %w", err)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
