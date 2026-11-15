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
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
)

// --- full-text search (Hauptplan E21, E101; PL26) ---
//
// Every type table has the column search (tsvector). records writes it in the transaction of
// every create and of every update that touches a searched field or a translation: the title
// field with weight A, the other searchable fields with weight B, both with the German
// dictionary, and their English translations with the English one. The job records.recompute
// fills it for the whole type when a package changes the searched fields, or for the first
// time. Search matches the column with websearch syntax (the last word also as a prefix) or the
// title by trigram similarity, over every type the session may read, with its rights.

// searchExpr is the SQL that computes the search column of the row it is evaluated on.
func searchExpr(t meta.Type) string {
	title, others := t.SearchFields()
	parts := searchParts(t, func(name string) string { return pgx.Identifier{name}.Sanitize() })
	var translated []string
	for _, name := range append([]string{title}, others...) {
		if f, ok := t.Field(name); ok && f.Translatable {
			translated = append(translated, "'"+name+"'")
		}
	}
	if len(translated) > 0 {
		// Type names and field names are identifiers (meta.IsIdent), safe as literals.
		parts = append(parts, fmt.Sprintf("setweight(to_tsvector('english', coalesce((SELECT string_agg(tr.value, ' ') FROM translation tr "+
			"WHERE tr.type_name = '%s' AND tr.record_id = %s.id AND tr.field IN (%s)), '')), 'B')",
			t.QualifiedName(), table(t), strings.Join(translated, ", ")))
	}
	if len(parts) == 0 {
		return "''::tsvector"
	}
	return strings.Join(parts, " || ")
}

// searchParts are the German parts of the search column, title weighted A and the other
// searched fields B, each read through col (a column, or an expression of a batch, PL32).
func searchParts(t meta.Type, col func(name string) string) []string {
	title, others := t.SearchFields()
	var parts []string
	text := func(name string) string {
		if f, _ := t.Field(name); f.Type == meta.FieldSequence {
			// A running number is found with and without its prefix (PL49).
			c := "(" + col(name) + ")::text"
			return "coalesce(" + quoteText(f.Prefix) + " || " + c + " || ' ' || " + c + ", '')"
		}
		return "coalesce((" + col(name) + ")::text, '')"
	}
	if title != "" {
		parts = append(parts, "setweight(to_tsvector('german', "+text(title)+"), 'A')")
	}
	for _, name := range others {
		parts = append(parts, "setweight(to_tsvector('german', "+text(name)+"), 'B')")
	}
	return parts
}

// searchOf is the search column from the German values alone, read through col: a new row of
// a bulk batch has no translations yet (PL32).
func searchOf(t meta.Type, col func(name string) string) string {
	parts := searchParts(t, col)
	if len(parts) == 0 {
		return "''::tsvector"
	}
	return strings.Join(parts, " || ")
}

// searchInsert returns the value of the search column of a new row, computed from its values
// and translations as text parameters appended to args; the same weights as searchExpr.
func searchInsert(t meta.Type, values map[string]any, tr translations, args []any) (string, []any) {
	title, others := t.SearchFields()
	param := func(name string) string {
		f, _ := t.Field(name)
		text := ""
		if v := toJSON(f, values[name]); v != nil {
			text = fmt.Sprint(v)
			if f.Type == meta.FieldSequence {
				text = f.Prefix + text + " " + text
			}
		}
		args = append(args, text)
		return fmt.Sprintf("$%d::text", len(args))
	}
	var parts, english []string
	if title != "" {
		parts = append(parts, "setweight(to_tsvector('german', "+param(title)+"), 'A')")
	}
	for _, name := range others {
		parts = append(parts, "setweight(to_tsvector('german', "+param(name)+"), 'B')")
	}
	for _, name := range append([]string{title}, others...) {
		if s, ok := tr[name]["en"].(string); ok && s != "" {
			english = append(english, s)
		}
	}
	if len(english) > 0 {
		args = append(args, strings.Join(english, " "))
		parts = append(parts, fmt.Sprintf("setweight(to_tsvector('english', $%d::text), 'B')", len(args)))
	}
	if len(parts) == 0 {
		return "''::tsvector", args
	}
	return strings.Join(parts, " || "), args
}

// refreshSearch computes the search column of one record.
func refreshSearch(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID) error {
	if !t.Searched() {
		return nil
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET search = %s WHERE id = $1", table(t), searchExpr(t)), id); err != nil {
		return fmt.Errorf("records: search column: %w", err)
	}
	return nil
}

// searchTouched reports whether a change concerns the search column.
func searchTouched(t meta.Type, changes []change) bool {
	if !t.Searched() {
		return false
	}
	title, others := t.SearchFields()
	for _, c := range changes {
		name, _, _ := strings.Cut(c.field, ".")
		if name == title {
			return true
		}
		for _, o := range others {
			if name == o {
				return true
			}
		}
	}
	return false
}

// RecomputeSearch fills the search column of every record of a type, in batches, and returns
// how many rows it wrote.
func (s *Service) RecomputeSearch(ctx context.Context, typeName string) (int, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return 0, err
	}
	if !t.Searched() {
		return 0, nil
	}
	scope := ""
	if c := listScope(t, ""); c != "" {
		scope = " AND " + c
	}
	sql := fmt.Sprintf(`UPDATE %[1]s SET search = %[2]s
WHERE id IN (SELECT id FROM %[1]s WHERE id > $1%[3]s ORDER BY id LIMIT %[4]d)
RETURNING id`, table(t), searchExpr(t), scope, recomputeBatch)
	total := 0
	after := uuid.Nil
	for {
		var last uuid.UUID
		n := 0
		err := s.tx(ctx, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, sql, after)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					return err
				}
				n++
				if strings.Compare(id.String(), last.String()) > 0 {
					last = id
				}
			}
			return rows.Err()
		})
		if err != nil {
			return total, fmt.Errorf("records: recompute search of %s: %w", typeName, err)
		}
		total += n
		if n < recomputeBatch {
			return total, nil
		}
		after = last
	}
}

// SearchUnfilled reports whether a type has records whose search column was never computed
// (records from before PL26); the package install queues the job for them.
func (s *Service) SearchUnfilled(ctx context.Context, typeName string) (bool, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil || !t.Searched() {
		return false, err
	}
	sql := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE search IS NULL", table(t))
	if c := listScope(t, ""); c != "" {
		sql += " AND " + c
	}
	var unfilled bool
	err = s.db.Tenant(ctx).QueryRow(ctx, sql+")").Scan(&unfilled)
	return unfilled, err
}

// SearchQuery asks the search.
type SearchQuery struct {
	Text  string
	Types []string // empty: every type the session may read
	Limit int      // default 20, at most 100
	// Any lets every word count instead of all (suggestions, SD18): the text of a form is
	// long, a record that shares some of its words is still worth showing, the more the
	// better. The text may then be longer; past MaxAnyText it is cut.
	Any bool
}

// MaxAnyText is the length of the text a search with Any takes.
const MaxAnyText = 2000

// The search looks at the newest records first (PL55, Kern-Plan E68): per type it takes the
// newest searchCandidates matches of the last searchRecent along the index on created_at and
// ranks only those. A frequent word ("Drucker" in a million of 18 million incidents) is then
// ranked over a thousand rows, not a million (found with H14: more than 10 s). Only when that
// brings fewer hits than asked for does it search every record, through the search index,
// where a rare word or an old number is cheap.
const (
	searchRecent     = 7 * 24 * time.Hour
	searchCandidates = 1000
)

// SearchHit is one record found.
type SearchHit struct {
	Type    string    `json:"type"`
	ID      uuid.UUID `json:"id"`
	Title   *string   `json:"title"`
	Rank    float64   `json:"rank"`
	Snippet string    `json:"snippet"`
}

// Search finds records over the types the session may read, best first.
func (s *Service) Search(ctx context.Context, q SearchQuery) ([]SearchHit, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return []SearchHit{}, nil
	}
	if r := []rune(text); q.Any && len(r) > MaxAnyText {
		text = string(r[:MaxAnyText])
	} else if !q.Any && len(r) > 200 {
		return nil, &ValidationError{Problems: []Problem{{Field: "q", Code: "max_length", Message: "must have at most 200 characters"}}}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)
	var types []meta.Type
	if len(q.Types) > 0 {
		for _, name := range q.Types {
			t, err := s.Type(ctx, name)
			if err != nil {
				return nil, err
			}
			types = append(types, t)
		}
	} else {
		all, err := s.Types(ctx)
		if err != nil {
			return nil, err
		}
		types = all
	}
	rest, last := prefixParts(text)
	args := []any{text}
	tsq := "(websearch_to_tsquery('german', $1) || websearch_to_tsquery('english', $1)"
	switch {
	case q.Any:
		// Every word: the AND of plainto_tsquery turned into OR; the last word also as the
		// start of a word, as it is being typed.
		tsq = "(" + anyWords("german") + " || " + anyWords("english")
		if last != "" {
			args = append(args, last)
			tsq += " || to_tsquery('german', $2)"
		}
	case last != "":
		args = append(args, rest, last)
		tsq += " || (websearch_to_tsquery('german', $2) && to_tsquery('german', $3))"
	}
	tsq += ")"
	// The newest first; only when they are too few, everything.
	since := time.Now().Add(-searchRecent)
	hits, err := s.search(ctx, types, tsq, args, limit, &since)
	if err != nil || len(hits) >= limit {
		return hits, err
	}
	return s.search(ctx, types, tsq, args, limit, nil)
}

// search runs the search over the types; with since, over the newest matches created after it.
func (s *Service) search(ctx context.Context, types []meta.Type, tsq string, args []any, limit int, since *time.Time) ([]SearchHit, error) {
	a := authz.From(ctx)
	args = slices.Clone(args)
	var subs []string
	for _, t := range types {
		if !t.Searched() || !a.Allowed(t.QualifiedName(), authz.ActionRead) {
			continue
		}
		title, others := t.SearchFields()
		readable := readableSet(ctx, t)
		// What the session may see of the record: the title and the readable searched fields.
		var docCols []string
		hidden := false
		for _, name := range append([]string{title}, others...) {
			if name == "" {
				continue
			}
			if !readable(name) {
				hidden = true
				continue
			}
			docCols = append(docCols, pgx.Identifier{name}.Sanitize()+"::text")
		}
		doc := "''"
		if len(docCols) > 0 {
			doc = "concat_ws(' ', " + strings.Join(docCols, ", ") + ")"
		}
		// The index serves who may read every searched field; the others match on what they see.
		match := "search @@ qq.q"
		if hidden {
			match = "to_tsvector('german', " + doc + ") @@ qq.q"
		}
		titleExpr, rank := "NULL::text", "coalesce(ts_rank(search, qq.q), 0)"
		if title != "" && readable(title) {
			col := pgx.Identifier{title}.Sanitize()
			titleExpr = col
			match = "(" + match + " OR " + col + " % $1)"
			rank = "coalesce(ts_rank(search, qq.q), 0) + similarity(" + col + ", $1)"
		}
		where := []string{"deleted_at IS NULL", match}
		if c := listScope(t, ""); c != "" {
			where = append(where, c)
		}
		var cond string
		cond, args = a.Filter(t.QualifiedName(), authz.ActionRead, args)
		where = append(where, cond)
		if since == nil {
			subs = append(subs, fmt.Sprintf("(SELECT '%s'::text AS type_name, id, %s AS title, %s AS doc, %s AS rank FROM %s, (SELECT %s) AS qq(q) WHERE %s ORDER BY rank DESC LIMIT %d)",
				t.QualifiedName(), titleExpr, doc, rank, table(t), tsq, strings.Join(where, " AND "), limit))
			continue
		}
		// The newest matches along the index on created_at, then ranked (PL55).
		args = append(args, *since)
		where = append(where, fmt.Sprintf("created_at >= $%d", len(args)))
		subs = append(subs, fmt.Sprintf("(SELECT '%s'::text AS type_name, id, %s AS title, %s AS doc, %s AS rank FROM (SELECT %s.* FROM %s, (SELECT %s) AS qq(q) WHERE %s ORDER BY created_at DESC LIMIT %d) AS %s, (SELECT %s) AS qq(q) ORDER BY rank DESC LIMIT %d)",
			t.QualifiedName(), titleExpr, doc, rank, table(t), table(t), tsq, strings.Join(where, " AND "), searchCandidates, table(t), tsq, limit))
	}
	if len(subs) == 0 {
		return []SearchHit{}, nil
	}
	sql := fmt.Sprintf(`SELECT type_name, id, title, rank,
       ts_headline('german', doc, %s, 'StartSel=<mark>, StopSel=</mark>, MaxFragments=1, MaxWords=15, MinWords=5')
FROM (%s) AS hits
ORDER BY rank DESC, id
LIMIT %d`, tsq, strings.Join(subs, " UNION ALL "), limit)
	// The search keeps the cache of plans (PL57, your decision 2026-11-15): planning its union
	// over every type anew cost some 8 ms a search, and on ki-lab it had no outliers.
	rows, err := s.db.Tenant(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("records: search: %w", err)
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var rank float32
		if err := rows.Scan(&h.Type, &h.ID, &h.Title, &rank, &h.Snippet); err != nil {
			return nil, err
		}
		h.Rank = float64(rank)
		out = append(out, h)
	}
	return out, rows.Err()
}

// prefixParts splits the search text for type-ahead: the text before the last word and the
// last word as a prefix query ("druck:*"). A last word in quotes, after a minus or with other
// characters than letters and digits is no prefix.
// anyWords is the query of the words of $1 in a dictionary, any of them matching.
func anyWords(dict string) string {
	return "replace(plainto_tsquery('" + dict + "', $1)::text, ' & ', ' | ')::tsquery"
}

func prefixParts(text string) (rest, last string) {
	fields := strings.Fields(text)
	if len(fields) == 0 || strings.Count(text, `"`)%2 == 1 || strings.HasSuffix(text, `"`) {
		return text, ""
	}
	word := fields[len(fields)-1]
	if len([]rune(word)) < 2 {
		return text, ""
	}
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return text, ""
		}
	}
	if strings.EqualFold(word, "or") {
		return text, ""
	}
	return strings.Join(fields[:len(fields)-1], " "), strings.ToLower(word) + ":*"
}
