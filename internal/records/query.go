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
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
)

// --- the JSON query (Hauptplan E40, E91; architektur.md §6.3; PL14) ---
//
// POST /api/v1/records/{type}:query takes a Query. where is a tree of and / or / not and
// conditions on a field; a field may be a path through reference fields ("parent.title").
// The query builder always adds the rights of the session and leaves the trash out; it never
// chooses indexes. Aggregates (group, aggregate) answer with rows instead of records.

// Query is the body of a query request.
type Query struct {
	Where     json.RawMessage      `json:"where,omitempty"`
	Fields    []string             `json:"fields,omitempty"`
	Sort      []string             `json:"sort,omitempty"`
	Limit     int                  `json:"limit,omitempty"`
	After     string               `json:"after,omitempty"`
	Count     bool                 `json:"count,omitempty"`
	Group     []string             `json:"group,omitempty"`
	Aggregate map[string]Aggregate `json:"aggregate,omitempty"`
	// Sets counts each group of Group on its own and all together in one pass over the
	// table (GROUPING SETS, PL67): the widgets of a dashboard with the same condition. Each
	// row carries SetKey: the index of its group in Group, or -1 for the total. Roles
	// (actors.<role>) do not take part; only for the code, not in the API.
	Sets bool `json:"-"`
	// RowLimit replaces the 1,000 rows of an aggregate (PL69: a month of day values per node
	// and group); only for the code, not in the API.
	RowLimit int `json:"-"`
}

// SetKey is the key of a row of a query with Sets: which group it belongs to.
const SetKey = "_set"

// Aggregate is one function over a field: {"count": "*"}, {"sum": "priority"}.
type Aggregate struct {
	Count string `json:"count,omitempty"`
	Sum   string `json:"sum,omitempty"`
	Min   string `json:"min,omitempty"`
	Max   string `json:"max,omitempty"`
	Avg   string `json:"avg,omitempty"`
}

// Rows is the answer of an aggregate query.
type Rows struct {
	Rows []map[string]any `json:"rows"`
}

// maxPathDepth bounds paths through references ("a.b.c").
const maxPathDepth = 3

// operators maps the JSON keys to SQL; "in", "nin", "contains", "starts", "ends" and "null"
// are built by hand.
var operators = map[string]string{"eq": "=", "neq": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}

// builder turns a Query into SQL for one type. alias names the table in a nested query
// (the rel operator), so that paths qualify the outer column correctly.
type builder struct {
	s        *Service
	ctx      context.Context
	t        meta.Type
	alias    string
	args     []any
	problems []Problem
}

func (b *builder) self() string {
	if b.alias != "" {
		return b.alias
	}
	return table(b.t)
}

func (b *builder) problem(field, code, msg string) {
	b.problems = append(b.problems, Problem{Field: field, Code: code, Message: msg})
}

func (b *builder) arg(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

// resolved is a field or a path as SQL: an expression, the field it ends in (or a system
// column) and whether it is a system column.
type resolved struct {
	expr   string
	field  meta.Field
	system string
}

// resolve turns "title" into "title" and "parent.title" into a scalar subquery on the
// referenced type, so that the base query needs no aliases (the rights condition names bare
// columns).
func (b *builder) resolve(path string) (resolved, bool) {
	parts := strings.Split(path, ".")
	if len(parts) > maxPathDepth+1 {
		b.problem(path, "path", fmt.Sprintf("at most %d references deep", maxPathDepth))
		return resolved{}, false
	}
	t := b.t
	var hops []meta.Field
	var hopTypes []meta.Type
	for i, p := range parts {
		last := i == len(parts)-1
		if f, ok := t.Field(p); ok {
			if last {
				return b.wrap(hops, hopTypes, resolved{expr: pgx.Identifier{p}.Sanitize(), field: f}), true
			}
			if f.Type != meta.FieldReference {
				b.problem(path, "path", fmt.Sprintf("%q is no reference, the path cannot go on", p))
				return resolved{}, false
			}
			next, err := b.s.Type(b.ctx, f.To)
			if err != nil {
				b.problem(path, "path", fmt.Sprintf("%q points to %s, which is not installed", p, f.To))
				return resolved{}, false
			}
			hops = append(hops, f)
			hopTypes = append(hopTypes, next)
			t = next
			continue
		}
		if last && (isSystem(p) || p == "state") {
			return b.wrap(hops, hopTypes, resolved{expr: pgx.Identifier{p}.Sanitize(), system: p}), true
		}
		b.problem(path, "unknown", fmt.Sprintf("%q is not a field of %s", p, t.QualifiedName()))
		return resolved{}, false
	}
	return resolved{}, false
}

// wrap nests the final column in one scalar subquery per hop, innermost last.
func (b *builder) wrap(hops []meta.Field, types []meta.Type, r resolved) resolved {
	expr := r.expr
	for i := len(hops) - 1; i >= 0; i-- {
		alias := fmt.Sprintf("h%d", i)
		// The outer column is qualified, because the referenced table may have a column of
		// the same name (a parent of the same type).
		outer := fmt.Sprintf("%s.%s", b.self(), pgx.Identifier{hops[i].Name}.Sanitize())
		if i > 0 {
			outer = fmt.Sprintf("h%d.%s", i-1, pgx.Identifier{hops[i].Name}.Sanitize())
		}
		inner := expr
		if i == len(hops)-1 {
			inner = alias + "." + expr
		}
		scope := ""
		if c := listScope(types[i], alias); c != "" {
			scope = " AND " + c
		}
		expr = fmt.Sprintf("(SELECT %s FROM %s %s WHERE %s.id = %s AND %s.deleted_at IS NULL%s)", inner, table(types[i]), alias, alias, outer, alias, scope)
	}
	r.expr = expr
	return r
}

// where builds the condition tree.
func (b *builder) where(raw json.RawMessage, depth int) string {
	if depth > 16 {
		b.problem("where", "depth", "the condition is nested too deep")
		return "TRUE"
	}
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		b.problem("where", "syntax", "a condition must be an object")
		return "TRUE"
	}
	if len(node) == 0 {
		return "TRUE"
	}
	for _, key := range []string{"and", "or"} {
		if sub, ok := node[key]; ok {
			var list []json.RawMessage
			if err := json.Unmarshal(sub, &list); err != nil || len(list) == 0 {
				b.problem("where", "syntax", key+" needs a non-empty list of conditions")
				return "TRUE"
			}
			parts := make([]string, 0, len(list))
			for _, item := range list {
				parts = append(parts, b.where(item, depth+1))
			}
			return "(" + strings.Join(parts, " "+strings.ToUpper(key)+" ") + ")"
		}
	}
	if sub, ok := node["not"]; ok {
		return "(NOT " + b.where(sub, depth+1) + ")"
	}
	if _, ok := node["rel"]; ok {
		return b.related(node, depth)
	}
	var field string
	if err := json.Unmarshal(node["field"], &field); err != nil || field == "" {
		b.problem("where", "syntax", "a condition needs \"field\" with an operator, or and / or / not")
		return "TRUE"
	}
	if isActorFilter(field) {
		return b.actors(field, node)
	}
	r, ok := b.resolve(field)
	if !ok {
		return "TRUE"
	}
	for key, raw := range node {
		if key == "field" {
			continue
		}
		return b.condition(field, r, key, raw)
	}
	b.problem("where", "syntax", fmt.Sprintf("the condition on %q has no operator", field))
	return "TRUE"
}

// value coerces a JSON value to the field's type (a system column takes it as is).
func (b *builder) value(field string, r resolved, v any) (any, bool) {
	if r.system != "" {
		if r.system == orgPathField {
			s, ok := v.(string)
			if !ok || !org.ValidPath(s) {
				b.problem(field, "type", "must be a path of the organisation tree, like root.it")
				return nil, false
			}
			return s, true
		}
		// "@now" is the moment of the query on a system time, so that a saved search for what
		// is overdue stays right (UI34).
		if s, ok := v.(string); ok && s == "@now" && strings.HasSuffix(r.system, "_at") {
			return time.Now().UTC(), true
		}
		return v, true
	}
	out, p := coerce(r.field, v)
	if p != nil {
		p.Field = field
		b.problems = append(b.problems, *p)
		return nil, false
	}
	return out, true
}

func (b *builder) condition(field string, r resolved, op string, raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		b.problem(field, "syntax", "bad value")
		return "TRUE"
	}
	isText := r.system != "" || r.field.Type == meta.FieldText || r.field.Type == meta.FieldChoice
	switch op {
	case "eq", "neq", "gt", "gte", "lt", "lte":
		val, ok := b.value(field, r, v)
		if !ok {
			return "TRUE"
		}
		if val == nil {
			if op == "eq" {
				return r.expr + " IS NULL"
			}
			if op == "neq" {
				return r.expr + " IS NOT NULL"
			}
			b.problem(field, "type", op+" needs a value")
			return "TRUE"
		}
		if r.system == orgPathField {
			return fmt.Sprintf("%s %s %s::ltree", r.expr, operators[op], b.arg(val))
		}
		return fmt.Sprintf("%s %s %s", r.expr, operators[op], b.arg(val))
	case "in", "nin":
		list, ok := v.([]any)
		if !ok || len(list) == 0 {
			b.problem(field, "type", op+" needs a non-empty list")
			return "TRUE"
		}
		ph := make([]string, 0, len(list))
		for _, item := range list {
			val, ok := b.value(field, r, item)
			if !ok {
				return "TRUE"
			}
			if val == nil {
				b.problem(field, "type", op+" takes no null")
				return "TRUE"
			}
			ph = append(ph, b.arg(val))
		}
		not := ""
		if op == "nin" {
			not = "NOT "
		}
		return fmt.Sprintf("%s%s IN (%s)", not, r.expr, strings.Join(ph, ", "))
	case "contains", "starts", "ends":
		s, ok := v.(string)
		if !ok || !isText {
			b.problem(field, "type", op+" needs a text field and a string")
			return "TRUE"
		}
		pattern := escapeLike(s)
		switch op {
		case "contains":
			pattern = "%" + pattern + "%"
		case "starts":
			pattern += "%"
		default:
			pattern = "%" + pattern
		}
		return fmt.Sprintf("%s ILIKE %s", r.expr, b.arg(pattern))
	case "null":
		isNull, ok := v.(bool)
		if !ok {
			b.problem(field, "type", "null needs true or false")
			return "TRUE"
		}
		if isNull {
			return r.expr + " IS NULL"
		}
		return r.expr + " IS NOT NULL"
	case "under":
		// {"field": "org_path", "under": "root.nord"}: the records at a unit and below it (UI13).
		if r.system == orgPathField {
			path, _ := v.(string)
			if !org.ValidPath(path) {
				b.problem(field, "type", "under on org_path needs a path of the organisation tree, like root.nord")
				return "TRUE"
			}
			return fmt.Sprintf("%s <@ %s::ltree", r.expr, b.arg(path))
		}
		// {"field": "parent", "under": id}: the entries below one entry of a tree list (PL16).
		s, _ := v.(string)
		id, err := uuid.Parse(s)
		if !b.t.IsTree() || field != "parent" || err != nil {
			b.problem(field, "type", "under needs the field parent of a tree list and the id of an entry")
			return "TRUE"
		}
		ref := b.arg(id)
		self := b.self()
		return fmt.Sprintf("(%s.path <@ (SELECT path FROM list_entry WHERE id = %s) AND %s.id <> %s)", self, ref, self, ref)
	}
	b.problem(field, "operator", fmt.Sprintf("%q is no operator; use eq, neq, gt, gte, lt, lte, in, nin, contains, starts, ends, null or under", op))
	return "TRUE"
}

// related builds {"rel": kind, "to": type, "direction": "out"|"in", "where": {...}}: an
// EXISTS over the relation table and the related type, with the rights of the session on
// the related type and its trash left out (PL15).
func (b *builder) related(node map[string]json.RawMessage, depth int) string {
	var kind, to, direction string
	_ = json.Unmarshal(node["rel"], &kind)
	_ = json.Unmarshal(node["to"], &to)
	_ = json.Unmarshal(node["direction"], &direction)
	if kind == "" || to == "" {
		b.problem("where", "rel", `"rel" needs the kind and "to" the related type`)
		return "TRUE"
	}
	target, err := b.s.Type(b.ctx, to)
	if err != nil {
		b.problem("where", "rel", fmt.Sprintf("%q is not an installed type", to))
		return "TRUE"
	}
	alias := fmt.Sprintf("rel%d", depth)
	near, far := "from", "to"
	switch direction {
	case "", "out":
	case "in":
		near, far = "to", "from"
	default:
		b.problem("where", "rel", "direction must be out or in")
		return "TRUE"
	}
	inner := &builder{s: b.s, ctx: b.ctx, t: target, alias: alias, args: b.args}
	cond, args := authz.From(b.ctx).Filter(target.QualifiedName(), authz.ActionRead, inner.args)
	inner.args = args
	where := []string{alias + ".deleted_at IS NULL", cond}
	if c := listScope(target, alias); c != "" {
		where = append(where, c)
	}
	if sub, ok := node["where"]; ok && len(sub) > 0 && string(sub) != "null" && string(sub) != "{}" {
		where = append(where, inner.where(sub, depth+1))
	}
	b.args = inner.args
	b.problems = append(b.problems, inner.problems...)
	kindArg := b.arg(kind)
	typeArg := b.arg(b.t.QualifiedName())
	toArg := b.arg(to)
	return fmt.Sprintf("EXISTS (SELECT 1 FROM %s %s WHERE %s.id IN (SELECT r.%s_id FROM relation r WHERE r.kind = %s AND r.%s_type = %s AND r.%s_id = %s.id AND r.%s_type = %s) AND %s)",
		table(target), alias, alias, far, kindArg, near, typeArg, near, b.self(), far, toArg, strings.Join(where, " AND "))
}

// actors builds a condition on the roles at the record (WF3): {"field": "actors.assigned",
// "eq": "@my_groups"}, "in" with a list of ids, @me and @my_groups, or "null": true for a role
// nobody holds. "actors" alone means any role.
func (b *builder) actors(field string, node map[string]json.RawMessage) string {
	for key, raw := range node {
		if key == "field" {
			continue
		}
		var values []string
		switch key {
		case "eq":
			var v string
			if json.Unmarshal(raw, &v) != nil {
				b.problem(field, "type", "eq needs an id, @me or @my_groups")
				return "TRUE"
			}
			values = []string{v}
		case "in":
			if json.Unmarshal(raw, &values) != nil || len(values) == 0 {
				b.problem(field, "type", "in needs a non-empty list of ids, @me or @my_groups")
				return "TRUE"
			}
		case "null":
			var isNull bool
			if json.Unmarshal(raw, &isNull) != nil {
				b.problem(field, "type", "null needs true or false")
				return "TRUE"
			}
			cond, p := actorHeld(b.t, field, b.self(), b.arg)
			if p != nil {
				b.problems = append(b.problems, *p)
				return "TRUE"
			}
			if isNull {
				return "NOT " + cond
			}
			return cond
		default:
			b.problem(field, "operator", fmt.Sprintf("%q is no operator on roles; use eq, in or null", key))
			return "TRUE"
		}
		cond, p := actorCondition(b.ctx, b.t, field, values, b.self(), b.arg)
		if p != nil {
			b.problems = append(b.problems, *p)
			return "TRUE"
		}
		return cond
	}
	b.problem("where", "syntax", fmt.Sprintf("the condition on %q has no operator", field))
	return "TRUE"
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// Query runs a JSON query: records (Page) or, with group / aggregate, Rows.
func (s *Service) Query(ctx context.Context, typeName string, q Query) (Page, *Rows, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return Page{}, nil, err
	}
	b := &builder{s: s, ctx: ctx, t: t}
	where := []string{"deleted_at IS NULL"}
	if c := listScope(t, ""); c != "" {
		where = append(where, c)
	}
	cond, args := authz.From(ctx).Filter(t.QualifiedName(), authz.ActionRead, nil)
	b.args = args
	where = append(where, cond)
	if len(q.Where) > 0 && string(q.Where) != "null" {
		where = append(where, b.where(q.Where, 0))
	}
	if len(q.Group) > 0 || len(q.Aggregate) > 0 {
		rows, err := s.aggregate(b, q, where)
		return Page{}, rows, err
	}
	return s.records(b, q, where)
}

// planned runs a list or an aggregate with a plan for its values (PL57, your decision
// 2026-11-15): pgx prepares statements, and after five runs PostgreSQL may switch to a
// generic plan that does not know the values; for the queue of a team on 18.25 million
// incidents that one hashed every actor row of the team (16 s instead of under 1 s, H14). An
// unnamed statement is planned with its values every time. Single records keep the cache.
func planned(args []any) []any {
	return append([]any{pgx.QueryExecModeCacheDescribe}, args...)
}

// neverEmpty are the system columns without empty values (NOT NULL in every type table).
var neverEmpty = map[string]bool{"version": true, "created_at": true, "updated_at": true}

// records selects records with sort, cursor and field selection.
func (s *Service) records(b *builder, q Query, where []string) (Page, *Rows, error) {
	t := b.t
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	// Sort: own columns only (paths cannot be paged by key set); the first one carries the
	// cursor, id breaks ties.
	sortCol, desc := "id", false
	if len(q.Sort) > 1 {
		b.problem("sort", "sort", "one sort column (the second is always id)")
	}
	if len(q.Sort) >= 1 {
		sortCol = strings.TrimPrefix(q.Sort[0], "-")
		desc = strings.HasPrefix(q.Sort[0], "-")
		if _, ok := t.Field(sortCol); !ok && !isSystem(sortCol) && !isExtra(t, sortCol) {
			b.problem("sort", "unknown", fmt.Sprintf("%q is not a field of %s", sortCol, t.QualifiedName()))
		}
		if strings.Contains(sortCol, ".") {
			b.problem("sort", "sort", "sorting by a path is not possible")
		}
	}
	sortExpr := pgx.Identifier{sortCol}.Sanitize()

	// Columns: own fields plus paths as extra columns named by their path.
	cols := columns(t)
	if len(q.Fields) > 0 {
		want := map[string]bool{"id": true, "version": true, sortCol: true}
		for _, f := range q.Fields {
			f = strings.TrimSpace(f)
			if strings.Contains(f, ".") {
				if _, ok := b.resolve(f); ok {
					cols = append(cols, f)
				}
				continue
			}
			if _, ok := t.Field(f); !ok && !isSystem(f) && !isExtra(t, f) {
				b.problem("fields", "unknown", fmt.Sprintf("%q is not a field of %s", f, t.QualifiedName()))
			}
			want[f] = true
		}
		var sel []string
		for _, c := range cols {
			if want[c] || strings.Contains(c, ".") {
				sel = append(sel, c)
			}
		}
		cols = sel
	}
	selects := make([]string, 0, len(cols))
	for _, c := range cols {
		if strings.Contains(c, ".") {
			r, _ := b.resolve(c)
			selects = append(selects, fmt.Sprintf("%s AS %s", r.expr, pgx.Identifier{c}.Sanitize()))
			continue
		}
		if c == actorsField && hasActors(t) {
			selects = append(selects, actorsExpr(t))
			continue
		}
		selects = append(selects, pgx.Identifier{c}.Sanitize())
	}

	var total *int64
	if q.Count && len(b.problems) == 0 {
		var n int64
		countSQL := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", table(t), strings.Join(where, " AND "))
		if err := s.db.Tenant(b.ctx).QueryRow(b.ctx, countSQL, planned(b.args)...).Scan(&n); err != nil {
			return Page{}, nil, fmt.Errorf("records: count %s: %w", t.QualifiedName(), err)
		}
		total = &n
	}

	// Cursor: (sort column, id) with NULLS LAST on both directions.
	if q.After != "" {
		var c cursor
		raw, err := base64.RawURLEncoding.DecodeString(q.After)
		if err == nil {
			err = json.Unmarshal(raw, &c)
		}
		if err != nil {
			b.problem("after", "cursor", "is not a cursor from a previous page")
		} else {
			op := ">"
			if desc {
				op = "<"
			}
			idArg := b.arg(c.ID)
			switch {
			case sortCol == "id":
				where = append(where, fmt.Sprintf("id %s %s", op, idArg))
			case neverEmpty[sortCol] && c.V != nil:
				// A column that is never empty compares as a row with id, which an index on it
				// serves directly (PL54).
				where = append(where, fmt.Sprintf("(%s, id) %s (%s, %s)", sortExpr, op, b.arg(c.V), idArg))
			case c.V == nil:
				where = append(where, fmt.Sprintf("(%s IS NULL AND id %s %s)", sortExpr, op, idArg))
			default:
				v := c.V
				if f, ok := t.Field(sortCol); ok {
					if cv, p := coerce(f, v); p == nil {
						v = cv
					}
				}
				vArg := b.arg(v)
				where = append(where, fmt.Sprintf("(%s %s %s OR (%s = %s AND id %s %s) OR %s IS NULL)", sortExpr, op, vArg, sortExpr, vArg, op, idArg, sortExpr))
			}
		}
	}
	if len(b.problems) > 0 {
		return Page{}, nil, &ValidationError{Problems: b.problems}
	}
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	order := fmt.Sprintf("%s %s NULLS LAST, id %s", sortExpr, dir, dir)
	switch {
	case sortCol == "id":
		order = "id " + dir
	case neverEmpty[sortCol]:
		// No NULLS LAST where there is no empty value: with it PostgreSQL cannot read the index
		// on the column backwards and sorts every hit instead (PL54, found with H14: the queue of
		// a team on 18.25 million incidents).
		order = fmt.Sprintf("%s %s, id %s", sortExpr, dir, dir)
	}
	sql := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d", strings.Join(selects, ", "), table(t), strings.Join(where, " AND "), order, limit+1)
	rows, err := s.db.Tenant(b.ctx).Query(b.ctx, sql, planned(b.args)...)
	if err != nil {
		return Page{}, nil, fmt.Errorf("records: query %s: %w", t.QualifiedName(), err)
	}
	defer rows.Close()
	page := Page{Total: total}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return Page{}, nil, err
		}
		rec := Record{}
		for i, c := range cols {
			if strings.Contains(c, ".") {
				r, _ := b.resolve(c)
				if r.system != "" {
					rec[c] = systemToJSON(r.system, vals[i])
				} else {
					rec[c] = toJSON(r.field, vals[i])
				}
				continue
			}
			if f, ok := t.Field(c); ok {
				rec[c] = toJSON(f, vals[i])
			} else if c == "version" {
				rec[c] = vals[i].(int64)
			} else if c == actorsField && hasActors(t) {
				rec[c] = actorsJSON(t, vals[i])
			} else {
				rec[c] = systemToJSON(c, vals[i])
			}
		}
		page.Items = append(page.Items, hideFields(b.ctx, t, rec))
	}
	if err := rows.Err(); err != nil {
		return Page{}, nil, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		c := cursor{ID: uuid.MustParse(last["id"].(string))}
		if sortCol != "id" {
			c.V = last[sortCol]
		}
		raw, _ := json.Marshal(c)
		page.Next = base64.RawURLEncoding.EncodeToString(raw)
	}
	if page.Items == nil {
		page.Items = []Record{}
	}
	return page, nil, nil
}

// aggregate groups and counts: SELECT group…, agg… GROUP BY group… ORDER BY group…
func (s *Service) aggregate(b *builder, q Query, where []string) (*Rows, error) {
	t := b.t
	var selects, groups []string
	var names []string
	type groupCol struct {
		r     resolved
		unit  string
		actor bool
	}
	var groupCols []groupCol
	// joins hold the roles a query groups by (UI14a): each the holders of one role, so that a
	// record with two groups counts for both and one without any for none (null).
	var joins []string
	for _, g := range q.Group {
		if isActorFilter(g) && q.Sets {
			b.problem("group", "invalid", fmt.Sprintf("%q: a role does not count in one pass with other groups", g))
			continue
		}
		if isActorFilter(g) {
			role, p := filterRole(t, g)
			switch {
			case p != nil:
				b.problems = append(b.problems, *p)
				continue
			case role == "":
				b.problem("group", "syntax", fmt.Sprintf("%q: group by one role, actors.<role>", g))
				continue
			}
			alias := fmt.Sprintf("grp_actor_%d", len(joins))
			joins = append(joins, fmt.Sprintf("LEFT JOIN (SELECT record_id AS rid, target_id AS target FROM actor WHERE type_name = %s AND role = %s) %s ON %s.rid = base.id",
				b.arg(t.QualifiedName()), b.arg(role), alias, alias))
			expr := alias + ".target"
			selects = append(selects, fmt.Sprintf("%s AS %s", expr, pgx.Identifier{g}.Sanitize()))
			groups = append(groups, expr)
			names = append(names, g)
			groupCols = append(groupCols, groupCol{actor: true})
			continue
		}
		// A date or a moment groups by day, week, month or year with "field:unit" (UI13):
		// "created_at:day", counted in Europe/Berlin.
		field, unit, _ := strings.Cut(g, ":")
		r, ok := b.resolve(field)
		if !ok {
			continue
		}
		expr := r.expr
		if unit != "" {
			if !slices.Contains([]string{"day", "week", "month", "year"}, unit) {
				b.problem("group", "syntax", fmt.Sprintf("%q: the unit is day, week, month or year", g))
				continue
			}
			switch {
			case r.system == "created_at" || r.system == "updated_at" || r.system == "deleted_at" || r.system == "sla_due_at" ||
				(r.system == "" && r.field.Type == meta.FieldDateTime):
				expr = fmt.Sprintf("date_trunc('%s', %s AT TIME ZONE 'Europe/Berlin')::date", unit, r.expr)
			case r.system == "" && r.field.Type == meta.FieldDate:
				expr = fmt.Sprintf("date_trunc('%s', %s)::date", unit, r.expr)
			default:
				b.problem("group", "type", fmt.Sprintf("%q: only a date or a moment groups by %s", g, unit))
				continue
			}
		}
		selects = append(selects, fmt.Sprintf("%s AS %s", expr, pgx.Identifier{g}.Sanitize()))
		groups = append(groups, expr)
		names = append(names, g)
		groupCols = append(groupCols, groupCol{r: r, unit: unit})
	}
	if len(q.Aggregate) == 0 {
		q.Aggregate = map[string]Aggregate{"count": {Count: "*"}}
	}
	aggNames := make([]string, 0, len(q.Aggregate))
	for name := range q.Aggregate {
		aggNames = append(aggNames, name)
	}
	sort.Strings(aggNames)
	type aggCol struct {
		name  string
		field *meta.Field
	}
	var aggCols []aggCol
	for _, name := range aggNames {
		a := q.Aggregate[name]
		fn, field := "", ""
		for f, v := range map[string]string{"count": a.Count, "sum": a.Sum, "min": a.Min, "max": a.Max, "avg": a.Avg} {
			if v != "" {
				if fn != "" {
					b.problem("aggregate", "syntax", fmt.Sprintf("%q has more than one function", name))
				}
				fn, field = f, v
			}
		}
		if fn == "" {
			b.problem("aggregate", "syntax", fmt.Sprintf("%q needs count, sum, min, max or avg", name))
			continue
		}
		if fn == "count" && field == "*" {
			selects = append(selects, fmt.Sprintf("count(*) AS %s", pgx.Identifier{name}.Sanitize()))
			aggCols = append(aggCols, aggCol{name: name})
			continue
		}
		r, ok := b.resolve(field)
		if !ok {
			continue
		}
		if fn != "count" && fn != "min" && fn != "max" && r.field.Type != meta.FieldNumber {
			b.problem("aggregate", "type", fmt.Sprintf("%s needs a number field, %q is %s", fn, field, r.field.Type))
			continue
		}
		selects = append(selects, fmt.Sprintf("%s(%s) AS %s", fn, r.expr, pgx.Identifier{name}.Sanitize()))
		f := r.field
		if fn == "count" {
			aggCols = append(aggCols, aggCol{name: name})
		} else {
			aggCols = append(aggCols, aggCol{name: name, field: &f})
		}
	}
	if len(b.problems) > 0 {
		return nil, &ValidationError{Problems: b.problems}
	}
	sql := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(selects, ", "), table(t), strings.Join(where, " AND "))
	if len(joins) > 0 {
		// The records readable first, then the holders of the roles: the names of the actor
		// table stay out of the conditions and the fields.
		sql = fmt.Sprintf("SELECT %s FROM (SELECT * FROM %s WHERE %s) base %s", strings.Join(selects, ", "), table(t),
			strings.Join(where, " AND "), strings.Join(joins, " "))
	}
	limit := 1000
	if q.RowLimit > 0 {
		limit = q.RowLimit
	}
	switch {
	case q.Sets:
		// One pass for the total and each group (PL67); GROUPING says which one a row is.
		sets := []string{"()"}
		for _, g := range groups {
			sets = append(sets, "("+g+")")
		}
		if len(groups) > 0 {
			sql = strings.Replace(sql, "SELECT ", fmt.Sprintf("SELECT GROUPING(%s), ", strings.Join(groups, ", ")), 1)
		} else {
			sql = strings.Replace(sql, "SELECT ", "SELECT 0, ", 1)
		}
		sql += " GROUP BY GROUPING SETS (" + strings.Join(sets, ", ") + ")"
		if len(groups) > 0 {
			sql += " ORDER BY 1 DESC, " + strings.Join(groups, ", ") + " NULLS LAST"
		}
		limit *= len(sets)
	case len(groups) > 0:
		sql += " GROUP BY " + strings.Join(groups, ", ") + " ORDER BY " + strings.Join(groups, ", ") + " NULLS LAST"
	}
	sql += fmt.Sprintf(" LIMIT %d", limit)
	rows, err := s.db.Tenant(b.ctx).Query(b.ctx, sql, planned(b.args)...)
	if err != nil {
		return nil, fmt.Errorf("records: aggregate %s: %w", t.QualifiedName(), err)
	}
	defer rows.Close()
	out := &Rows{Rows: []map[string]any{}}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := map[string]any{}
		if q.Sets {
			// GROUPING has a bit for each group, set when the row is not grouped by it: all
			// set is the total, all but one the group of that one.
			mask, _ := vals[0].(int32)
			vals = vals[1:]
			row[SetKey] = -1
			for i := range names {
				if mask&(1<<(len(names)-1-i)) == 0 {
					row[SetKey] = i
				}
			}
		}
		for i, g := range names {
			gc := groupCols[i]
			switch {
			case gc.actor:
				row[g] = toJSON(meta.Field{Type: meta.FieldReference}, vals[i])
			case gc.unit != "":
				if d, ok := vals[i].(time.Time); ok {
					row[g] = d.Format(dateFormat)
				} else {
					row[g] = nil
				}
			case gc.r.system != "":
				row[g] = systemToJSON(gc.r.system, vals[i])
			default:
				row[g] = toJSON(gc.r.field, vals[i])
			}
		}
		for i, a := range aggCols {
			v := vals[len(names)+i]
			if a.field != nil {
				v = toJSON(*a.field, v)
			} else {
				v = toJSON(meta.Field{Type: meta.FieldNumber}, v)
			}
			// Counts and sums are numbers like every other number of the API.
			if n, ok := v.(int64); ok {
				v = float64(n)
			}
			row[a.name] = v
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}
