// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

// --- a batch of bulk rows, set-based (PL32; Hauptplan E38; architektur.md §6.4) ---
//
// SyncBatch does for many rows what Sync does for one, with a few statements instead of a
// transaction per row: every row is checked in Go as before (types, required fields, rights,
// formulas; organisation nodes and references are looked up once per batch and value), then
// the rows go by COPY into a temporary table, and one INSERT … ON CONFLICT DO NOTHING, one
// UPDATE … FROM and two COPY into audit and event write them. Rows the set-based way does not
// cover go the way of Sync: a tree list (its paths), translated values given as an object of
// languages, a move to another node of the organisation tree, and every row the statements
// left out (a key or a unique value taken meanwhile, a version changed meanwhile). A batch
// whose statements fail on a constraint is done row by row, so that the faulty row gets the
// error and the others are written.

// BulkRow is one row of a batch: the source's key, the delivered fields, their checksum.
type BulkRow struct {
	Key    string
	Fields map[string]any
	Hash   []byte
}

// BulkResult is what happened to one row: a Sync outcome, or Err (a validation error,
// ErrForbidden, ErrNotFound or ErrConflict).
type BulkResult struct {
	Status string
	ID     uuid.UUID
	Err    error
}

// bulkTemp is the temporary table of a batch.
const bulkTemp = "bulk_rows"

// pending is a row the set-based statements write.
type pending struct {
	i       int
	op      string // c create, u update, h hash only
	id      uuid.UUID
	version int64 // the version read (update)
	path    string
	values  map[string]any // values to write: all fields (create), the changed ones (update)
	changes []change
	old     Record
}

// SyncBatch writes rows of one source and returns a result per row, in their order. An error
// is one of the database; the rows written before it stay.
func (s *Service) SyncBatch(ctx context.Context, typeName, source string, rows []BulkRow, actor Actor) ([]BulkResult, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	results := make([]BulkResult, len(rows))
	ctx = withLookups(ctx)
	if t.IsTree() {
		return results, s.syncRows(ctx, t, source, rows, results, allRows(len(rows)), actor)
	}
	known, err := s.bulkExisting(ctx, t, source, rows)
	if err != nil {
		return nil, err
	}
	var (
		todo []pending
		slow []int // rows for the way of Sync
	)
	for i, row := range rows {
		p, res, ok := s.bulkPrepare(ctx, t, row, known[row.Key], actor)
		switch {
		case res != nil:
			p.i = i
			results[i] = *res
		case !ok:
			slow = append(slow, i)
		default:
			p.i = i
			todo = append(todo, p)
		}
	}
	if len(todo) > 0 {
		left, err := s.bulkWrite(ctx, t, source, rows, todo, results, actor)
		if err != nil {
			return nil, err
		}
		slow = append(slow, left...)
	}
	return results, s.syncRows(ctx, t, source, rows, results, slow, actor)
}

func allRows(n int) []int {
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	return all
}

// syncRows writes rows one by one, like PL31.
func (s *Service) syncRows(ctx context.Context, t meta.Type, source string, rows []BulkRow, results []BulkResult, which []int, actor Actor) error {
	s.rowByRow.Add(int64(len(which)))
	for _, i := range which {
		row := rows[i]
		id, _, status, err := s.Sync(ctx, t.QualifiedName(), SyncKey{Source: source, Key: row.Key, Hash: row.Hash}, row.Fields, actor)
		if err != nil && !rowFault(err) {
			return err
		}
		results[i] = BulkResult{Status: status, ID: id, Err: err}
	}
	return nil
}

// rowFault tells an error of the row from one of the database.
func rowFault(err error) bool {
	if _, ok := AsValidationError(err); ok {
		return true
	}
	return errors.Is(err, ErrForbidden) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotAllowed)
}

// existing is a record a source delivered before.
type existing struct {
	rec     Record
	hash    []byte
	trashed bool
}

// bulkExisting reads the records the batch's keys already have.
func (s *Service) bulkExisting(ctx context.Context, t meta.Type, source string, rows []BulkRow) (map[string]existing, error) {
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.Key
	}
	cols := columns(t)
	// A batch stays in the tenant of the request; the keys are unique there (PL46).
	sql := fmt.Sprintf("SELECT %s, row_hash, deleted_at IS NOT NULL FROM %s WHERE source = $1 AND external_key = ANY($2) AND tenant = tenant_of($3::ltree)",
		strings.Join(selected(t, cols), ", "), table(t))
	if c := listScope(t, ""); c != "" {
		sql += " AND " + c
	}
	res, err := s.db.Tenant(ctx).Query(ctx, sql, source, keys, authz.From(ctx).HomeNode())
	if err != nil {
		return nil, fmt.Errorf("records: bulk read %s: %w", t.QualifiedName(), err)
	}
	defer res.Close()
	out := map[string]existing{}
	for res.Next() {
		vals, err := res.Values()
		if err != nil {
			return nil, err
		}
		n := len(cols)
		rec := toRecord(t, cols, vals[:n])
		hash, _ := vals[n].([]byte)
		key, _ := rec["external_key"].(string)
		out[key] = existing{rec: rec, hash: hash, trashed: vals[n+1].(bool)}
	}
	return out, res.Err()
}

// bulkPrepare checks one row. It returns the row to write, or a finished result, or ok=false
// for the way of Sync.
func (s *Service) bulkPrepare(ctx context.Context, t meta.Type, row BulkRow, ex existing, actor Actor) (pending, *BulkResult, bool) {
	fail := func(err error) (pending, *BulkResult, bool) {
		return pending{}, &BulkResult{Status: "", Err: err}, true
	}
	if _, given := row.Fields[actorsField]; given {
		// Roles at the record go the way of Sync (WF3).
		return pending{}, nil, false
	}
	input, tr, tps := splitTranslations(ctx, t, row.Fields)
	if len(tr) > 0 || s.timed(ctx, t) || s.rolledUp(ctx, t) {
		// Translations, the clocks of a type with SLAs (WF9) and the sums of a type others
		// count (WF13) go the way of Sync.
		return pending{}, nil, false
	}
	if ex.rec == nil {
		values, err := s.validate(ctx, t, input, true)
		if err = withProblems(err, tps); err != nil {
			return fail(err)
		}
		path, err := createNode(ctx, t, values)
		if err != nil {
			return fail(err)
		}
		if s.rulesWould(ctx, t, TriggerCreate, apiShape(t, Record{stateField: t.InitialState(), orgPathField: path}, values), changedNames(t, nil, values, nil)) {
			// Rules before the write (WF4) go the way of Sync, with the history naming them.
			return pending{}, nil, false
		}
		if err := createStates(t, input, values, nil); err != nil {
			return fail(err)
		}
		if err := s.check(ctx, t, apiShape(t, Record{stateField: t.InitialState(), orgPathField: path}, values)); err != nil {
			return fail(err)
		}
		if err := checkCreate(ctx, t, path, values); err != nil {
			return fail(err)
		}
		if err := checkFields(ctx, t, authz.ActionCreate, values); err != nil {
			return fail(err)
		}
		state := make(map[string]any, len(values))
		for k, v := range values {
			state[k] = v
		}
		if err := calculate(t, state, values); err != nil {
			return fail(err)
		}
		id, err := uuid.NewV7()
		if err != nil {
			return fail(err)
		}
		return pending{op: "c", id: id, path: path, values: values}, nil, true
	}

	id, _ := uuid.Parse(fmt.Sprint(ex.rec["id"]))
	version, _ := ex.rec["version"].(int64)
	switch {
	case ex.trashed:
		return fail(fmt.Errorf("%w: the record of key %q is in the trash; restore it first", ErrConflict, row.Key))
	case bytes.Equal(ex.hash, row.Hash):
		if err := checkAccess(ctx, t, ex.rec, authz.ActionUpdate); err != nil {
			return fail(err)
		}
		return pending{}, &BulkResult{Status: SyncUnchanged, ID: id}, true
	}
	values, err := s.validate(ctx, t, input, false)
	if err = withProblems(err, tps); err != nil {
		return fail(err)
	}
	if p, ok := values[orgPathField]; ok && p != ex.rec[orgPathField] {
		return pending{}, nil, false
	}
	if err := checkAccess(ctx, t, ex.rec, authz.ActionUpdate); err != nil {
		return fail(err)
	}
	if err := checkFields(ctx, t, authz.ActionUpdate, values); err != nil {
		return fail(err)
	}
	if ch := changedNames(t, ex.rec, values, nil); len(ch) > 0 && s.rulesWould(ctx, t, TriggerUpdate, apiShape(t, ex.rec, values), ch) {
		return pending{}, nil, false
	}
	if err := changeStates(t, ex.rec, apiShape(t, ex.rec, values), false, nil); err != nil {
		return fail(err)
	}
	if err := s.check(ctx, t, apiShape(t, ex.rec, values)); err != nil {
		return fail(err)
	}
	state := make(map[string]any, len(ex.rec)+len(values))
	for k, v := range ex.rec {
		state[k] = v
	}
	for k, v := range values {
		state[k] = v
	}
	if err := calculate(t, state, values); err != nil {
		return fail(err)
	}
	p := pending{op: "h", id: id, version: version, path: fmt.Sprint(ex.rec[orgPathField]), values: map[string]any{}, old: ex.rec}
	for _, f := range t.Fields {
		v, ok := values[f.Name]
		if !ok {
			continue
		}
		newJSON := toJSON(f, v)
		if equalJSON(ex.rec[f.Name], newJSON) {
			continue
		}
		p.values[f.Name] = v
		p.changes = append(p.changes, change{f.Name, ex.rec[f.Name], newJSON})
	}
	if len(p.changes) > 0 {
		p.op = "u"
	}
	return p, nil, true
}

// bulkWrite writes the prepared rows in one transaction and returns the rows the statements
// left out, for the way of Sync.
func (s *Service) bulkWrite(ctx context.Context, t meta.Type, source string, rows []BulkRow, todo []pending, results []BulkResult, actor Actor) ([]int, error) {
	var left []int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := copyTemp(ctx, tx, t, rows, todo); err != nil {
			return err
		}
		inserted, err := idSet(ctx, tx, insertSQL(t), uuidOrNil(actor), source)
		if err != nil {
			return err
		}
		updated, err := idSet(ctx, tx, updateSQL(t), uuidOrNil(actor))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s t SET row_hash = u._hash FROM %s u WHERE u._op = 'h' AND t.id = u._id::uuid", table(t), bulkTemp)); err != nil {
			return fmt.Errorf("records: bulk hash: %w", err)
		}
		var searchIDs []uuid.UUID
		var audits, events [][]any
		at := time.Now()
		for _, p := range todo {
			switch {
			case p.op == "c" && inserted[p.id]:
				results[p.i] = BulkResult{Status: SyncCreated, ID: p.id}
				audits, events = createdRows(t, p, actor, audits, events)
			case p.op == "u" && updated[p.id]:
				results[p.i] = BulkResult{Status: SyncUpdated, ID: p.id}
				audits, events = updatedRows(t, p, actor, audits, events)
				if searchTouched(t, p.changes) {
					searchIDs = append(searchIDs, p.id)
				}
			case p.op == "h":
				results[p.i] = BulkResult{Status: SyncUnchanged, ID: p.id}
			default:
				left = append(left, p.i)
			}
		}
		if len(searchIDs) > 0 {
			if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET search = %s WHERE id = ANY($1)", table(t), searchExpr(t)), searchIDs); err != nil {
				return fmt.Errorf("records: bulk search: %w", err)
			}
		}
		if t.HasHistory() && len(audits) > 0 {
			for _, a := range audits {
				a[len(a)-1] = at
			}
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"audit"},
				[]string{"id", "type_name", "record_id", "version", "action", "field", "old_value", "new_value", "actor", "at"},
				pgx.CopyFromRows(audits)); err != nil {
				return fmt.Errorf("records: bulk audit: %w", err)
			}
		}
		if len(events) > 0 {
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"event"}, []string{"kind", "type_name", "record_id", "actor", "payload"},
				pgx.CopyFromRows(events)); err != nil {
				return fmt.Errorf("records: bulk events: %w", err)
			}
		}
		return nil
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && strings.HasPrefix(pg.Code, "23") {
		// A constraint refused the batch (a unique value twice in it, for instance): row by
		// row, the faulty row gets its error.
		for _, p := range todo {
			results[p.i] = BulkResult{}
		}
		return allIndexes(todo), nil
	}
	if err != nil {
		return nil, err
	}
	s.afterWrite(ctx, t.QualifiedName())
	return left, nil
}

func allIndexes(todo []pending) []int {
	out := make([]int, len(todo))
	for i, p := range todo {
		out[i] = p.i
	}
	return out
}

// tempCol is the column of a field in the temporary table; the prefix keeps field names apart
// from the batch's own columns.
func tempCol(name string) string { return pgx.Identifier{"f_" + name}.Sanitize() }

// writable are the fields a batch writes: all but the ones the core keeps.
func writable(t meta.Type) []meta.Field {
	var out []meta.Field
	for _, f := range t.Fields {
		if !f.Computed {
			out = append(out, f)
		}
	}
	return out
}

// copyTemp creates the temporary table of the batch and fills it by COPY: every value as text,
// cast to the column type when written.
func copyTemp(ctx context.Context, tx pgx.Tx, t meta.Type, rows []BulkRow, todo []pending) error {
	fields := writable(t)
	defs := []string{"_op text", "_id text", "_version bigint", "_path text", "_key text", "_hash bytea", "_changed text[]"}
	cols := []string{"_op", "_id", "_version", "_path", "_key", "_hash", "_changed"}
	for _, f := range fields {
		defs = append(defs, tempCol(f.Name)+" text")
		cols = append(cols, "f_"+f.Name)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("CREATE TEMP TABLE %s (%s) ON COMMIT DROP", bulkTemp, strings.Join(defs, ", "))); err != nil {
		return fmt.Errorf("records: bulk temp: %w", err)
	}
	data := make([][]any, 0, len(todo))
	for _, p := range todo {
		changed := make([]string, 0, len(p.changes))
		for _, c := range p.changes {
			changed = append(changed, c.field)
		}
		row := []any{p.op, p.id.String(), p.version, p.path, rows[p.i].Key, rows[p.i].Hash, changed}
		for _, f := range fields {
			row = append(row, asText(p.values[f.Name]))
		}
		data = append(data, row)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{bulkTemp}, cols, pgx.CopyFromRows(data)); err != nil {
		return fmt.Errorf("records: bulk copy: %w", err)
	}
	return nil
}

// asText is a checked value as text for the temporary table; nil stays NULL.
func asText(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case uuid.UUID:
		return x.String()
	}
	return fmt.Sprint(v)
}

// cast is a column of the temporary table as the type of the field.
func cast(f meta.Field, col string) string {
	typ, err := schema.ColumnType(f.Type)
	if err != nil {
		typ = "text"
	}
	if f.Type == meta.FieldDate {
		// A date travels as a timestamp of midnight UTC.
		return fmt.Sprintf("(%s::timestamptz AT TIME ZONE 'UTC')::date", col)
	}
	return col + "::" + typ
}

func insertSQL(t meta.Type) string {
	cols := []string{"id", "org_path", "created_by", "updated_by", "source", "external_key", "row_hash"}
	sel := []string{"u._id::uuid", "u._path::ltree", "$1::uuid", "$1::uuid", "$2", "u._key", "u._hash"}
	if t.HasStates() {
		cols = append(cols, "state")
		sel = append(sel, "'"+strings.ReplaceAll(t.InitialState(), "'", "''")+"'")
	}
	for _, f := range writable(t) {
		cols = append(cols, f.Name)
		sel = append(sel, cast(f, "u."+tempCol(f.Name)))
	}
	if t.IsList() {
		cols = append(cols, "list")
		sel = append(sel, "'"+strings.ReplaceAll(t.QualifiedName(), "'", "''")+"'")
	}
	if t.Searched() {
		// New rows have no translations: the search column from the values alone.
		cols = append(cols, "search")
		sel = append(sel, searchOf(t, func(name string) string {
			f, _ := t.Field(name)
			return cast(f, "u."+tempCol(name))
		}))
	}
	return fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s u WHERE u._op = 'c' ON CONFLICT DO NOTHING RETURNING id",
		table(t), strings.Join(quoted(cols), ", "), strings.Join(sel, ", "), bulkTemp)
}

func updateSQL(t meta.Type) string {
	var sets []string
	for _, f := range writable(t) {
		name := pgx.Identifier{f.Name}.Sanitize()
		sets = append(sets, fmt.Sprintf("%s = CASE WHEN '%s' = ANY(u._changed) THEN %s ELSE t.%s END", name, f.Name, cast(f, "u."+tempCol(f.Name)), name))
	}
	sets = append(sets, "row_hash = u._hash", "version = t.version + 1", "updated_by = $1::uuid", "updated_at = now()")
	sql := fmt.Sprintf("UPDATE %s t SET %s FROM %s u WHERE u._op = 'u' AND t.id = u._id::uuid AND t.version = u._version AND t.deleted_at IS NULL",
		table(t), strings.Join(sets, ", "), bulkTemp)
	if c := listScope(t, "t"); c != "" {
		sql += " AND " + c
	}
	return sql + " RETURNING t.id"
}

// idSet runs a statement that returns ids.
func idSet(ctx context.Context, tx pgx.Tx, sql string, args ...any) (map[uuid.UUID]bool, error) {
	res, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	out := map[uuid.UUID]bool{}
	for res.Next() {
		var id uuid.UUID
		if err := res.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, res.Err()
}

// createdRows adds the audit rows and the event of a new record, as Create writes them.
func createdRows(t meta.Type, p pending, actor Actor, audits, events [][]any) ([][]any, [][]any) {
	audits = append(audits, auditRow(t, p.id, 1, "create", nil, actor))
	changed := []string{}
	for _, f := range t.Fields {
		if f.Computed {
			continue
		}
		if v := toJSON(f, p.values[f.Name]); v != nil {
			audits = append(audits, auditRow(t, p.id, 1, "update", &change{f.Name, nil, v}, actor))
			changed = append(changed, f.Name)
		}
	}
	var createdBy any
	if actor != nil {
		createdBy = (*actor).String()
	}
	return audits, append(events, eventRow(t, p.id, "create", map[string]any{"version": int64(1), "changed": changed, "org_path": p.path, "created_by": createdBy}, actor))
}

// updatedRows adds the audit rows and the event of a changed record, as Update writes them.
func updatedRows(t meta.Type, p pending, actor Actor, audits, events [][]any) ([][]any, [][]any) {
	v := p.version + 1
	changed := make([]string, 0, len(p.changes))
	for i := range p.changes {
		audits = append(audits, auditRow(t, p.id, v, "update", &p.changes[i], actor))
		changed = append(changed, p.changes[i].field)
	}
	return audits, append(events, eventRow(t, p.id, "update", map[string]any{"version": v, "changed": changed, "org_path": p.old[orgPathField], "created_by": p.old["created_by"]}, actor))
}

// auditRow is a row for COPY into audit; the last value, at, is set when the batch writes.
func auditRow(t meta.Type, id uuid.UUID, version int64, action string, c *change, actor Actor) []any {
	aid, _ := uuid.NewV7()
	var field, oldV, newV any
	if c != nil {
		field = c.field
		o, _ := json.Marshal(c.old)
		n, _ := json.Marshal(c.new)
		oldV, newV = string(o), string(n)
	}
	return []any{aid, t.QualifiedName(), id, version, action, field, oldV, newV, uuidOrNil(actor), nil}
}

func eventRow(t meta.Type, id uuid.UUID, kind string, payload map[string]any, actor Actor) []any {
	raw, _ := json.Marshal(payload)
	return []any{kind, t.QualifiedName(), id, uuidOrNil(actor), string(raw)}
}

// --- lookups once per batch ---

type lookupCache struct {
	mu    sync.Mutex
	nodes map[string]bool
	refs  map[string]bool
}

type lookupKey struct{}

// withLookups remembers, for the rest of ctx, which nodes and referenced records exist.
func withLookups(ctx context.Context) context.Context {
	return context.WithValue(ctx, lookupKey{}, &lookupCache{nodes: map[string]bool{}, refs: map[string]bool{}})
}

func (s *Service) nodeKnown(ctx context.Context, path string) (bool, error) {
	c, _ := ctx.Value(lookupKey{}).(*lookupCache)
	if c == nil {
		return s.nodeExists(ctx, path)
	}
	c.mu.Lock()
	ok, seen := c.nodes[path]
	c.mu.Unlock()
	if seen {
		return ok, nil
	}
	ok, err := s.nodeExists(ctx, path)
	if err == nil {
		c.mu.Lock()
		c.nodes[path] = ok
		c.mu.Unlock()
	}
	return ok, err
}

func (s *Service) refKnown(ctx context.Context, typeName string, id uuid.UUID) (bool, error) {
	c, _ := ctx.Value(lookupKey{}).(*lookupCache)
	if c == nil {
		return s.exists(ctx, typeName, id)
	}
	key := typeName + "/" + id.String()
	c.mu.Lock()
	ok, seen := c.refs[key]
	c.mu.Unlock()
	if seen {
		return ok, nil
	}
	ok, err := s.exists(ctx, typeName, id)
	if err == nil {
		c.mu.Lock()
		c.refs[key] = ok
		c.mu.Unlock()
	}
	return ok, err
}
