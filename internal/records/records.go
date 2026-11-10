// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package records reads and writes the rows of entity types (architektur.md §2 "Datensätze",
// §3.1, §6.2). A record is a flat map: system fields next to the fields of the type (Hauptplan
// E70). Every write is one transaction with validation, the row, the audit rows and (later) the
// outbox event. Writes carry the version the caller saw; a stale version is a conflict (E29).
//
// This package and query are the only ones allowed to read or write type tables (E34).
package records

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/schema"
)

// Record is the API shape: system fields and type fields side by side.
type Record map[string]any

// Actor is who performs a write; nil until authentication (PL5) supplies one.
type Actor *uuid.UUID

// System fields every record carries in the API, in output order.
var systemFields = []string{"id", "version", "state", "org_path", "source", "external_key",
	"created_at", "created_by", "updated_at", "updated_by", "deleted_at",
	"sla_due_at", "sla_warn_at", "sla_paused"}

// Service performs record operations for the installed types.
type Service struct {
	db    *db.Router
	types func(ctx context.Context) (map[string]meta.Type, error)
	// seqs are the sequences of the running numbers known to exist, by pool and name (PL49).
	seqs sync.Map
	// nodeExists tells whether a path of the organisation tree exists (PL6).
	nodeExists func(ctx context.Context, path string) (bool, error)
	// AfterWrite, when set, is told after every successful write which type changed; the
	// rights cache uses it for group memberships (PL9), the outbox dispatcher later (PL24).
	AfterWrite func(ctx context.Context, typeName string)
	// Settings reads a setting of a node (org.Service.Setting); the costs read their currency
	// and hourly rate through it (PL20). Nil means the defaults.
	Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)
	// rowByRow counts the bulk rows a batch wrote the way of Sync instead of set-based (PL32);
	// tests read it.
	rowByRow atomic.Int64
	// Before is the rule engine's hook before a create or update is written (WF4); nil means
	// no rules.
	Before BeforeWrite
	// Clocks is the hook of the SLA clocks (WF9); nil means none.
	Clocks Clocks
	// CheckRecord checks records beyond their fields (WF9: the conditions of an SLA); nil means
	// none.
	CheckRecord RecordCheck
}

func (s *Service) afterWrite(ctx context.Context, typeName string) {
	if s.AfterWrite != nil {
		s.AfterWrite(ctx, typeName)
	}
}

// written reports a finished write to AfterWrite and passes the error through.
func (s *Service) written(ctx context.Context, typeName string, err error) error {
	if err == nil {
		s.afterWrite(ctx, typeName)
	}
	return err
}

// New creates a service that reads the type definitions through q (cached later, PL9).
func New(rt *db.Router) *Service {
	return &Service{db: rt, nodeExists: func(ctx context.Context, path string) (bool, error) {
		if !org.ValidPath(path) {
			return false, nil
		}
		return queries.New(rt.Tenant(ctx)).OrgNodeExists(ctx, path)
	}, types: func(ctx context.Context) (map[string]meta.Type, error) {
		ts, err := meta.Types(ctx, queries.New(rt.Control()))
		if err != nil {
			return nil, err
		}
		m := make(map[string]meta.Type, len(ts))
		for _, t := range ts {
			m[t.QualifiedName()] = t
		}
		return m, nil
	}}
}

// Type returns the definition of a qualified type name.
func (s *Service) Type(ctx context.Context, typeName string) (meta.Type, error) {
	ts, err := s.types(ctx)
	if err != nil {
		return meta.Type{}, err
	}
	t, ok := ts[typeName]
	if !ok {
		return meta.Type{}, fmt.Errorf("type %s: %w", typeName, ErrNotFound)
	}
	return t, nil
}

// Get reads one record outside the trash.
func (s *Service) Get(ctx context.Context, typeName string, id uuid.UUID) (Record, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	rec, err := s.read(ctx, s.db.Tenant(ctx), t, id, false, false)
	if err != nil {
		return nil, err
	}
	if err := checkAccess(ctx, t, rec, authz.ActionRead); err != nil {
		return nil, err
	}
	return hideFields(ctx, t, rec), nil
}

// Check reads a record (outside the trash) and applies the rights for an action: not
// readable is ErrNotFound, readable but not allowed is ErrForbidden (relations, PL15).
func (s *Service) Check(ctx context.Context, typeName string, id uuid.UUID, action string) (Record, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	rec, err := s.read(ctx, s.db.Tenant(ctx), t, id, false, false)
	if err != nil {
		return nil, err
	}
	if err := checkAccess(ctx, t, rec, action); err != nil {
		return nil, err
	}
	return rec, nil
}

// Trace writes an audit row and an event for something that happened to a record without
// changing its row (a relation added or removed, PL15): the audit carries the action and
// the payload as new value, the event the payload.
func (s *Service) Trace(ctx context.Context, typeName string, id uuid.UUID, action string, payload map[string]any, actor Actor) error {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		return trace(ctx, tx, s, t, id, action, payload, actor)
	})
	if err == nil {
		s.afterWrite(ctx, t.QualifiedName())
	}
	return err
}

// trace is Trace in a transaction.
func trace(ctx context.Context, tx pgx.Tx, s *Service, t meta.Type, id uuid.UUID, action string, payload map[string]any, actor Actor) error {
	rec, err := s.read(ctx, tx, t, id, true, false)
	if err != nil {
		return err
	}
	q := queries.New(tx)
	version := rec["version"].(int64)
	field := "relation"
	switch action {
	case "attach", "detach":
		field = "documents"
	case "notify":
		// A notification of a rule (WF4).
		field = "notify"
	case "breach":
		// A clock past its deadline (WF9).
		field = "clocks"
	}
	if err := audit(ctx, q, t, id, version, action, &change{field, nil, payload}, actor); err != nil {
		return err
	}
	return event(ctx, q, t, id, rec, action, version, []string{}, payload, actor)
}

// Page is a key-set page of records (architektur.md §6.1: pages by key, never by offset).
type Page struct {
	Items []Record `json:"items"`
	// Next is the cursor to pass as "after" for the next page; empty on the last page.
	Next string `json:"next,omitempty"`
	// Total is the number of matching records when the caller asked for it.
	Total *int64 `json:"total,omitempty"`
}

// ListOptions narrow and order a list.
type ListOptions struct {
	Limit int
	// After is the Next cursor of the previous page.
	After string
	// Sort names one column, "-" prefix for descending: id, created_at, updated_at, version or
	// sla_due_at, the deadline (UI34), whose records without one come last. Default is id.
	Sort string
	// Fields limits the returned fields; id and version always come.
	Fields []string
	// Filter holds equality conditions on fields or on state, source, external_key.
	Filter map[string]string
	// Count asks for the number of records the filters match (all pages), as Total.
	Count bool
}

// sortable lists the columns a list can be ordered by with a key-set cursor.
var sortable = map[string]bool{"id": true, "created_at": true, "updated_at": true, "version": true, "sla_due_at": true}

// nullable are the sortable columns that may be empty; their empty rows come last.
var nullable = map[string]bool{"sla_due_at": true}

// filterableSystem lists the system columns a list can be filtered by.
var filterableSystem = map[string]bool{"state": true, "source": true, "external_key": true}

type cursor struct {
	V  any       `json:"v"`
	ID uuid.UUID `json:"id"`
}

// List returns records outside the trash.
func (s *Service) List(ctx context.Context, typeName string, o ListOptions) (Page, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return Page{}, err
	}
	limit := o.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var ps []Problem

	// Sort column and direction.
	sortCol, desc := "id", false
	if o.Sort != "" {
		sortCol = strings.TrimPrefix(o.Sort, "-")
		desc = strings.HasPrefix(o.Sort, "-")
		if !sortable[sortCol] {
			ps = append(ps, Problem{Field: "sort", Code: "sort", Message: fmt.Sprintf("%q cannot be sorted by; use id, created_at, updated_at, version or sla_due_at", sortCol)})
		}
	}

	// Columns to return.
	cols := columns(t)
	if len(o.Fields) > 0 {
		want := map[string]bool{"id": true, "version": true}
		for _, f := range o.Fields {
			f = strings.TrimSpace(f)
			if _, ok := t.Field(f); !ok && !isSystem(f) && !isExtra(t, f) {
				ps = append(ps, Problem{Field: "fields", Code: "unknown", Message: fmt.Sprintf("%q is not a field of %s", f, t.QualifiedName())})
			}
			want[f] = true
		}
		want[sortCol] = true
		var sel []string
		for _, c := range cols {
			if want[c] {
				sel = append(sel, c)
			}
		}
		cols = sel
	}

	// The rights of the request (PL8): a condition on org_path and created_by, or TRUE.
	where := []string{"deleted_at IS NULL"}
	if c := listScope(t, ""); c != "" {
		where = append(where, c)
	}
	var args []any
	cond, args := authz.From(ctx).Filter(t.QualifiedName(), authz.ActionRead, args)
	where = append(where, cond)

	// Filters, in name order so that the SQL is stable.
	names := make([]string, 0, len(o.Filter))
	for name := range o.Filter {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := o.Filter[name]
		if isActorFilter(name) {
			// Who holds a role (WF3): ids, @me or @my_groups, separated by commas.
			cond, p := actorCondition(ctx, t, name, strings.Split(raw, ","), table(t), func(v any) string {
				args = append(args, v)
				return fmt.Sprintf("$%d", len(args))
			})
			if p != nil {
				ps = append(ps, *p)
				continue
			}
			where = append(where, cond)
			continue
		}
		if f, ok := t.Field(name); ok {
			v, p := coerceString(f, raw)
			if p != nil {
				ps = append(ps, *p)
				continue
			}
			args = append(args, v)
		} else if name == orgPathField {
			// The subtree below a node (architektur.md §5): org_path <@ node, on the GiST index.
			if !org.ValidPath(raw) {
				ps = append(ps, Problem{Field: name, Code: "type", Message: "must be a path of the organisation tree, like root.it"})
				continue
			}
			args = append(args, raw)
			where = append(where, fmt.Sprintf("org_path <@ $%d::ltree", len(args)))
			continue
		} else if name == "under" && t.IsTree() {
			// The subtree below an entry of a tree list (PL16).
			id, err := uuid.Parse(raw)
			if err != nil {
				ps = append(ps, Problem{Field: name, Code: "type", Message: "must be the id of an entry"})
				continue
			}
			args = append(args, id)
			where = append(where, underCondition(fmt.Sprintf("$%d", len(args))))
			continue
		} else if filterableSystem[name] {
			args = append(args, raw)
		} else {
			ps = append(ps, Problem{Field: name, Code: "unknown", Message: fmt.Sprintf("%q cannot be filtered by", name)})
			continue
		}
		where = append(where, fmt.Sprintf("%s = $%d", pgx.Identifier{name}.Sanitize(), len(args)))
	}

	var total *int64
	if o.Count && len(ps) == 0 {
		var n int64
		countSQL := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", table(t), strings.Join(where, " AND "))
		if err := s.db.Tenant(ctx).QueryRow(ctx, countSQL, args...).Scan(&n); err != nil {
			return Page{}, fmt.Errorf("records: count %s: %w", t.QualifiedName(), err)
		}
		total = &n
	}

	// Cursor.
	if o.After != "" {
		var c cursor
		b, err := base64.RawURLEncoding.DecodeString(o.After)
		if err == nil {
			err = json.Unmarshal(b, &c)
		}
		if err != nil {
			ps = append(ps, Problem{Field: "after", Code: "cursor", Message: "is not a cursor from a previous page"})
		} else {
			op := ">"
			if desc {
				op = "<"
			}
			col := pgx.Identifier{sortCol}.Sanitize()
			switch {
			case sortCol == "id":
				args = append(args, c.ID)
				where = append(where, fmt.Sprintf("id %s $%d", op, len(args)))
			case nullable[sortCol] && c.V == nil:
				// The page ended among the empty ones, which come last.
				args = append(args, c.ID)
				where = append(where, fmt.Sprintf("(%s IS NULL AND id %s $%d)", col, op, len(args)))
			case nullable[sortCol]:
				args = append(args, c.V, c.ID)
				where = append(where, fmt.Sprintf("((%s, id) %s ($%d, $%d) OR %s IS NULL)", col, op, len(args)-1, len(args), col))
			default:
				args = append(args, c.V, c.ID)
				where = append(where, fmt.Sprintf("(%s, id) %s ($%d, $%d)", col, op, len(args)-1, len(args)))
			}
		}
	}
	if len(ps) > 0 {
		return Page{}, &ValidationError{Problems: ps}
	}

	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	order := fmt.Sprintf("%s %s", pgx.Identifier{sortCol}.Sanitize(), dir)
	if nullable[sortCol] {
		order += " NULLS LAST"
	}
	if sortCol != "id" {
		order += ", id " + dir
	}
	sql := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d",
		strings.Join(selected(t, cols), ", "), table(t), strings.Join(where, " AND "), order, limit+1)
	rows, err := s.db.Tenant(ctx).Query(ctx, sql, args...)
	if err != nil {
		return Page{}, fmt.Errorf("records: list %s: %w", typeName, err)
	}
	defer rows.Close()
	page := Page{Total: total}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, hideFields(ctx, t, toRecord(t, cols, vals)))
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		c := cursor{ID: uuid.MustParse(last["id"].(string))}
		if sortCol != "id" {
			c.V = last[sortCol]
		}
		b, _ := json.Marshal(c)
		page.Next = base64.RawURLEncoding.EncodeToString(b)
	}
	return page, nil
}

// Traits returns the core traits and the installed traits of the packages (PL17).
func (s *Service) Traits(ctx context.Context) ([]meta.Trait, error) {
	return meta.Traits(ctx, queries.New(s.db.Control()))
}

// Types returns the installed types in name order.
func (s *Service) Types(ctx context.Context) ([]meta.Type, error) {
	m, err := s.types(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]meta.Type, 0, len(m))
	for _, t := range m {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QualifiedName() < out[j].QualifiedName() })
	return out, nil
}

// Create validates the input, applies defaults and inserts the row with its audit rows.
func (s *Service) Create(ctx context.Context, typeName string, input map[string]any, actor Actor) (Record, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, t, input, actor, nil, nil)
}

// CreateWithEntry is Create with the first entry of the timeline in the same transaction (the
// free answers of a form, UI11): record and entry stand or fall together. The entry belongs
// to the creation, so it needs no right comment beyond create; it is never private.
func (s *Service) CreateWithEntry(ctx context.Context, typeName string, input, entry map[string]any, actor Actor) (Record, error) {
	if entry == nil {
		return s.Create(ctx, typeName, input, actor)
	}
	t, err := s.timelineType(ctx, typeName)
	if err != nil {
		return nil, err
	}
	body, format, private, err := entryValues(entry)
	if err != nil {
		return nil, err
	}
	if private {
		return nil, &ValidationError{Problems: []Problem{{Field: "private", Code: "invalid", Message: "the first entry of a new record is not private"}}}
	}
	return s.create(ctx, t, input, actor, nil, &Entry{Body: body, Format: format})
}

// create is Create on a loaded type; key, when set, is the source, external key and row hash
// of a bulk row (PL31), written with the record; first, when set, is the first entry of the
// timeline (UI11).
func (s *Service) create(ctx context.Context, t meta.Type, input map[string]any, actor Actor, key *SyncKey, first *Entry) (Record, error) {
	input, rawActors, setsActors := splitActors(input)
	input, tr, tps := splitTranslations(ctx, t, input)
	actors, aps, err := s.actorsOf(ctx, t, rawActors, setsActors)
	if err != nil {
		return nil, err
	}
	// An import with a source may bring the running numbers (PL49).
	var given map[string]int64
	if key != nil {
		var nps []Problem
		input, given, nps = takeNumbers(t, input)
		aps = append(aps, nps...)
	}
	values, err := s.validate(ctx, t, input, true)
	if err = withProblems(err, append(tps, aps...)); err != nil {
		return nil, err
	}
	for f, n := range given {
		values[f] = n
	}
	// The caller's field rights, before rules add their values with the rights of the system.
	if err := checkFields(ctx, t, authz.ActionCreate, withActors(withTranslated(values, tr), nil, t, actors)); err != nil {
		return nil, err
	}
	node, err := createNode(ctx, t, values)
	if err != nil {
		return nil, err
	}
	// Rules before the write (WF4) see the record as it will be.
	planned := withActors(apiShape(t, Record{stateField: t.InitialState(), orgPathField: node}, values), nil, t, actors)
	actors, by, err := s.applyRules(ctx, t, TriggerCreate, planned, values, actors, changedNames(t, nil, values, actorChanges(t, nil, actors)))
	if err != nil {
		return nil, err
	}
	// Field states of the initial state (WF2): presets, read-only, required.
	if err := createStates(t, input, values, by); err != nil {
		return nil, err
	}
	// Checks beyond the fields, such as the conditions of an SLA (WF9).
	if err := s.check(ctx, t, apiShape(t, Record{stateField: t.InitialState(), orgPathField: node}, values)); err != nil {
		return nil, err
	}
	if err := checkCreate(ctx, t, node, withActors(values, nil, t, actors)); err != nil {
		return nil, err
	}
	actorCh := actorChanges(t, nil, actors)
	// Calculated fields from the record as it will be written (PL22).
	state := make(map[string]any, len(values))
	for k, v := range values {
		state[k] = v
	}
	if err := calculate(t, state, values); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	if fixed, ok := ctx.Value(fixedIDKey{}).(uuid.UUID); ok {
		// A mirror keeps the id of its original (Mirror, PL47b).
		id = fixed
	}
	seqs, err := s.sequences(ctx, t, node)
	if err != nil {
		return nil, err
	}
	var rec Record
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The running numbers in the transaction of the write (PL49): a new one from the
		// sequence, an imported one moves the sequence above it.
		for f, seq := range seqs {
			if n, ok := given[f]; ok {
				if err := raiseSequence(ctx, tx, seq, n); err != nil {
					return err
				}
				continue
			}
			ns, err := s.nextNumbers(ctx, tx, seq, 1)
			if err != nil {
				return err
			}
			values[f] = ns[0]
		}
		cols := []string{"id", "created_by", "updated_by"}
		args := []any{id, uuidOrNil(actor), uuidOrNil(actor)}
		if t.HasStates() {
			cols = append(cols, "state")
			args = append(args, t.InitialState())
		}
		cols = append(cols, orgPathField)
		args = append(args, node)
		for _, f := range t.Fields {
			if v, ok := values[f.Name]; ok {
				cols = append(cols, f.Name)
				args = append(args, v)
			}
		}
		if t.IsList() {
			cols = append(cols, "list")
			args = append(args, t.QualifiedName())
		}
		if key != nil {
			cols = append(cols, "source", "external_key", "row_hash")
			args = append(args, key.Source, key.Key, key.Hash)
		}
		ph := make([]string, len(args))
		for i := range args {
			ph[i] = fmt.Sprintf("$%d", i+1)
		}
		if t.Searched() {
			// The search column in the same statement, from the values (PL26).
			var expr string
			expr, args = searchInsert(t, values, tr, args)
			cols = append(cols, "search")
			ph = append(ph, expr)
		}
		sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table(t), strings.Join(quoted(cols), ", "), strings.Join(ph, ", "))
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return mapDBError(t, err)
		}
		if t.IsTree() {
			if err := treeRefresh(ctx, tx, id); err != nil {
				return err
			}
		}
		if err := writeActors(ctx, queries.New(tx), t, id, actors, actorCh, actor); err != nil {
			return err
		}
		var err error
		rec, err = s.read(ctx, tx, t, id, false, false)
		if err != nil {
			return err
		}
		q := queries.New(tx)
		if err := audit(ctx, q, t, id, 1, "create", nil, actor); err != nil {
			return err
		}
		changed := []string{}
		for _, f := range t.Fields {
			// Computed fields (the sums of the costs) start at zero and are no change.
			if v := rec[f.Name]; v != nil && !f.Computed {
				if err := audit(withRuleName(ctx, by[f.Name]), q, t, id, 1, "update", &change{f.Name, nil, v}, actor); err != nil {
					return err
				}
				changed = append(changed, f.Name)
			}
		}
		// Translations of the new record (PL21).
		trChanges := translationChanges(tr, nil)
		if err := writeTranslations(ctx, q, t.QualifiedName(), id, trChanges); err != nil {
			return err
		}
		for i := range trChanges {
			if err := audit(ctx, q, t, id, 1, "update", &trChanges[i], actor); err != nil {
				return err
			}
			changed = append(changed, trChanges[i].field)
		}
		// The roles of the new record (WF3).
		for i := range actorCh {
			if err := audit(withRuleName(ctx, by[actorCh[i].field]), q, t, id, 1, "update", &actorCh[i], actor); err != nil {
				return err
			}
			changed = append(changed, actorCh[i].field)
		}
		if err := event(ctx, q, t, id, rec, "create", 1, changed, rulesOf(by), actor); err != nil {
			return err
		}
		if first != nil {
			if first.ID, err = uuid.NewV7(); err != nil {
				return err
			}
			at, err := q.InsertEntry(ctx, queries.InsertEntryParams{ID: first.ID, TypeName: t.QualifiedName(), RecordID: id,
				Body: first.Body, Format: first.Format, CreatedBy: nullActor(actor)})
			if err != nil {
				return fmt.Errorf("records: first entry: %w", err)
			}
			first.CreatedAt, first.CreatedBy = at.Time, actor
			if err := event(ctx, q, t, id, rec, EventComment, 1, []string{},
				map[string]any{"entry": first.ID.String(), "private": false}, actor); err != nil {
				return err
			}
		}
		// The sums at the record it points to (WF13), then its clocks (WF9).
		if err := s.rollupsWritten(ctx, tx, t, nil, rec, actor); err != nil {
			return err
		}
		return s.clocksWritten(ctx, tx, t, id, rec)
	})
	if err == nil {
		s.afterWrite(ctx, t.QualifiedName())
	}
	return rec, err
}

// Update changes the given fields of a record that still has expectedVersion. Fields not in
// patch stay as they are; a field set to null is cleared.
func (s *Service) Update(ctx context.Context, typeName string, id uuid.UUID, expectedVersion int64, patch map[string]any, actor Actor) (Record, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, t, id, expectedVersion, patch, actor, nil, nil)
}

// update is Update on a loaded type; key, when set, stores the row hash of a bulk row (PL31),
// also when no field changes: then without a new version, audit or event. move, when set,
// is a transition the write makes together with the fields (WF1, WF2): its state and
// condition are checked against the record as it will be, the state changes in the same
// version, and the event is the transition. expectedVersion below 0 takes any version.
func (s *Service) update(ctx context.Context, t meta.Type, id uuid.UUID, expectedVersion int64, patch map[string]any, actor Actor, key *SyncKey, move *meta.Transition) (Record, error) {
	patch, rawActors, setsActors := splitActors(patch)
	patch, tr, tps := splitTranslations(ctx, t, patch)
	actors, aps, err := s.actorsOf(ctx, t, rawActors, setsActors)
	if err != nil {
		return nil, err
	}
	// An import sends the running number again with every row; it may not change it (PL49).
	var kept map[string]int64
	if key != nil {
		var nps []Problem
		patch, kept, nps = takeNumbers(t, patch)
		aps = append(aps, nps...)
	}
	values, err := s.validate(ctx, t, patch, false)
	if err = withProblems(err, append(tps, aps...)); err != nil {
		return nil, err
	}
	var rec Record
	var by map[string]string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		old, err := s.read(ctx, tx, t, id, false, true)
		if err != nil {
			return err
		}
		if err := checkAccess(ctx, t, old, authz.ActionUpdate); err != nil {
			return err
		}
		for f, n := range kept {
			if had, _ := old[f].(int64); had != n {
				return &ValidationError{Problems: []Problem{{Field: f, Code: "read_only",
					Message: fmt.Sprintf("is the running number %d and cannot be changed", had)}}}
			}
		}
		if err := checkFields(ctx, t, authz.ActionUpdate, withActors(withTranslated(values, tr), old, t, actors)); err != nil {
			return err
		}
		if expectedVersion < 0 {
			expectedVersion = old["version"].(int64)
		}
		if old["version"].(int64) != expectedVersion {
			return fmt.Errorf("%w: record %s is at version %d, you sent %d", ErrConflict, id, old["version"], expectedVersion)
		}
		after := apiShape(t, old, values)
		if move != nil {
			if err := moveAllowed(*move, after); err != nil {
				return err
			}
			after[stateField] = move.To
		}
		// Rules before the write (WF4), when the write changes anything: they see the record
		// as it will be and the names of what changes.
		callerActors := len(actorChanges(t, old, actors)) > 0
		changing := changedNames(t, old, values, actorChanges(t, old, actors))
		if move != nil {
			changing = append(changing, stateField)
		}
		if len(changing) > 0 || len(tr) > 0 {
			actors, by, err = s.applyRules(ctx, t, TriggerUpdate, withActors(after, old, t, actors), values, actors, changing)
			if err != nil {
				return err
			}
			if len(by) > 0 {
				after = apiShape(t, old, values)
				if move != nil {
					after[stateField] = move.To
				}
			}
		}
		// Field states (WF2): read-only in the state now, required in the state after.
		if err := changeStates(t, old, after, callerActors, by); err != nil {
			return err
		}
		if err := s.check(ctx, t, after); err != nil {
			return err
		}
		// Calculated fields from the old record with the new values (PL22).
		state := make(map[string]any, len(old)+len(values))
		for k, v := range old {
			state[k] = v
		}
		for k, v := range values {
			state[k] = v
		}
		if err := calculate(t, state, values); err != nil {
			return err
		}
		var sets []string
		args := []any{id, expectedVersion}
		var changes []change
		for _, f := range t.Fields {
			v, ok := values[f.Name]
			if !ok {
				continue
			}
			newJSON := toJSON(f, v)
			if equalJSON(old[f.Name], newJSON) {
				continue
			}
			args = append(args, v)
			sets = append(sets, fmt.Sprintf("%s = $%d", pgx.Identifier{f.Name}.Sanitize(), len(args)))
			changes = append(changes, change{f.Name, old[f.Name], newJSON})
		}
		if p, ok := values[orgPathField]; ok && old[orgPathField] != p {
			if err := checkCreate(ctx, t, p.(string), values); err != nil {
				return err
			}
			args = append(args, p)
			sets = append(sets, fmt.Sprintf("%s = $%d::ltree", orgPathField, len(args)))
			changes = append(changes, change{orgPathField, old[orgPathField], p})
		}
		if move != nil {
			args = append(args, move.To)
			sets = append(sets, fmt.Sprintf("%s = $%d", stateField, len(args)))
			changes = append(changes, change{stateField, old[stateField], move.To})
		}
		if len(tr) > 0 {
			stored, err := loadTranslations(ctx, queries.New(tx), t.QualifiedName(), []uuid.UUID{id})
			if err != nil {
				return err
			}
			changes = append(changes, translationChanges(tr, stored[id])...)
		}
		changes = append(changes, actorChanges(t, old, actors)...)
		if len(changes) == 0 {
			rec = old
			if key != nil {
				return storeRowHash(ctx, tx, t, id, key.Hash)
			}
			return nil
		}
		if t.IsTree() {
			if err := checkTreeParent(ctx, tx, id, values); err != nil {
				return err
			}
		}
		if key != nil {
			args = append(args, key.Hash)
			sets = append(sets, fmt.Sprintf("row_hash = $%d", len(args)))
		}
		args = append(args, uuidOrNil(actor))
		sets = append(sets, fmt.Sprintf("updated_by = $%d", len(args)), "updated_at = now()", "version = version + 1")
		sql := fmt.Sprintf("UPDATE %s SET %s WHERE id = $1 AND version = $2 AND deleted_at IS NULL", table(t), strings.Join(sets, ", "))
		tag, err := tx.Exec(ctx, sql, args...)
		if err != nil {
			return mapDBError(t, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("%w: record %s changed meanwhile", ErrConflict, id)
		}
		var trChanges []change
		for _, c := range changes {
			if isTranslation(c) {
				trChanges = append(trChanges, c)
			}
		}
		if err := writeTranslations(ctx, queries.New(tx), t.QualifiedName(), id, trChanges); err != nil {
			return err
		}
		if err := writeActors(ctx, queries.New(tx), t, id, actors, changes, actor); err != nil {
			return err
		}
		if searchTouched(t, changes) {
			if err := refreshSearch(ctx, tx, t, id); err != nil {
				return err
			}
		}
		if t.IsTree() && treeChanged(changes) {
			if err := treeRefresh(ctx, tx, id); err != nil {
				return err
			}
		}
		rec, err = s.read(ctx, tx, t, id, false, false)
		if err != nil {
			return err
		}
		q := queries.New(tx)
		v := rec["version"].(int64)
		changed := make([]string, 0, len(changes))
		for _, c := range changes {
			action := "update"
			if move != nil && c.field == stateField {
				action = "transition"
			}
			if err := audit(withRuleName(ctx, by[c.field]), q, t, id, v, action, &c, actor); err != nil {
				return err
			}
			changed = append(changed, c.field)
		}
		extra := rulesOf(by)
		if move != nil {
			if extra == nil {
				extra = map[string]any{}
			}
			extra["transition"], extra["from"], extra["to"] = move.Name, old[stateField], move.To
			if err := event(ctx, q, t, id, rec, "transition", v, changed, extra, actor); err != nil {
				return err
			}
		} else if err := event(ctx, q, t, id, rec, "update", v, changed, extra, actor); err != nil {
			return err
		}
		// The text a transition took along (SD-Plan N8): an entry of the same write.
		if body, _ := ctx.Value(commentKey{}).(string); move != nil && body != "" {
			entry, err := uuid.NewV7()
			if err != nil {
				return err
			}
			if _, err := q.InsertEntry(ctx, queries.InsertEntryParams{ID: entry, TypeName: t.QualifiedName(), RecordID: id,
				Body: body, Format: FormatText, CreatedBy: nullActor(actor)}); err != nil {
				return fmt.Errorf("records: entry of %s: %w", move.Name, err)
			}
			if err := event(ctx, q, t, id, rec, EventComment, v, []string{}, map[string]any{"entry": entry.String(), "private": false}, actor); err != nil {
				return err
			}
		}
		// The sums at the records it points to, before and after (WF13); the clocks pause, go
		// on or stop with the record as it is now (WF9).
		if err := s.rollupsWritten(ctx, tx, t, old, rec, actor); err != nil {
			return err
		}
		return s.clocksWritten(ctx, tx, t, id, rec)
	})
	if err == nil {
		s.afterWrite(ctx, t.QualifiedName())
	}
	return rec, err
}

// Trash moves a record to the trash (soft delete). It stays readable with Restore.
func (s *Service) Trash(ctx context.Context, typeName string, id uuid.UUID, actor Actor) error {
	return s.written(ctx, typeName, s.mark(ctx, typeName, id, actor, "trash", -1))
}

// TrashVersion moves a record to the trash if it still has expectedVersion (UI6).
func (s *Service) TrashVersion(ctx context.Context, typeName string, id uuid.UUID, expectedVersion int64, actor Actor) error {
	return s.written(ctx, typeName, s.mark(ctx, typeName, id, actor, "trash", expectedVersion))
}

// Restore takes a record out of the trash. A unique value taken meanwhile is a conflict.
func (s *Service) Restore(ctx context.Context, typeName string, id uuid.UUID, actor Actor) error {
	return s.written(ctx, typeName, s.mark(ctx, typeName, id, actor, "restore", -1))
}

// mark trashes or restores a record; expectedVersion below 0 takes the version it has.
func (s *Service) mark(ctx context.Context, typeName string, id uuid.UUID, actor Actor, action string, expectedVersion int64) error {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		old, err := s.read(ctx, tx, t, id, true, true)
		if err != nil {
			return err
		}
		if err := checkAccess(ctx, t, old, authz.ActionDelete); err != nil {
			return err
		}
		if v := old["version"].(int64); expectedVersion >= 0 && v != expectedVersion {
			return fmt.Errorf("%w: record %s is at version %d, you sent %d", ErrConflict, id, v, expectedVersion)
		}
		inTrash := old["deleted_at"] != nil
		if (action == "trash") == inTrash {
			if inTrash {
				return fmt.Errorf("%w: record %s is already in the trash", ErrConflict, id)
			}
			return fmt.Errorf("%w: record %s is not in the trash", ErrConflict, id)
		}
		set := "deleted_at = now()"
		if action == "restore" {
			set = "deleted_at = NULL"
		}
		sql := fmt.Sprintf("UPDATE %s SET %s, updated_at = now(), updated_by = $2, version = version + 1 WHERE id = $1", table(t), set)
		if _, err := tx.Exec(ctx, sql, id, uuidOrNil(actor)); err != nil {
			return mapDBError(t, err)
		}
		q := queries.New(tx)
		version := old["version"].(int64) + 1
		if err := audit(ctx, q, t, id, version, action, nil, actor); err != nil {
			return err
		}
		if err := event(ctx, q, t, id, old, action, version, []string{}, nil, actor); err != nil {
			return err
		}
		if !s.timed(ctx, t) && !s.rolledUp(ctx, t) {
			return nil
		}
		// In the trash a record does not count in sums (WF13) and its clocks hold; restored,
		// it counts again and they go on as the record says (WF9).
		rec, err := s.read(ctx, tx, t, id, true, false)
		if err != nil {
			return err
		}
		if err := s.rollupsWritten(ctx, tx, t, nil, rec, actor); err != nil {
			return err
		}
		return s.clocksWritten(ctx, tx, t, id, rec)
	})
}

// Purge deletes a record in the trash for good.
func (s *Service) Purge(ctx context.Context, typeName string, id uuid.UUID, actor Actor) error {
	return s.written(ctx, typeName, s.purge(ctx, typeName, id, actor))
}

func (s *Service) purge(ctx context.Context, typeName string, id uuid.UUID, actor Actor) error {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		old, err := s.read(ctx, tx, t, id, true, true)
		if err != nil {
			return err
		}
		if err := checkAccess(ctx, t, old, authz.ActionDelete); err != nil {
			return err
		}
		if old["deleted_at"] == nil {
			return fmt.Errorf("%w: record %s must be in the trash before it is purged", ErrConflict, id)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", table(t)), id); err != nil {
			return mapDBError(t, err)
		}
		// The clocks go with the record (WF9); the sums it counted in no longer had it since
		// the trash (WF13).
		if err := s.clocksWritten(ctx, tx, t, id, nil); err != nil {
			return err
		}
		q := queries.New(tx)
		if err := purgeTranslations(ctx, tx, t.QualifiedName(), id); err != nil {
			return err
		}
		if t.HasTrait(meta.TraitCosts) {
			// The cost entries go with the record (PL20); the history keeps the purge.
			if err := q.DeleteCostsOfRecord(ctx, queries.DeleteCostsOfRecordParams{TypeName: t.QualifiedName(), RecordID: id}); err != nil {
				return fmt.Errorf("records: purge costs: %w", err)
			}
		}
		// The timeline goes with the record (PL42), also of a type that no longer has the trait.
		if err := q.DeleteEntriesOfRecord(ctx, queries.DeleteEntriesOfRecordParams{TypeName: t.QualifiedName(), RecordID: id}); err != nil {
			return fmt.Errorf("records: purge entries: %w", err)
		}
		// So do the ratings (SD18).
		if err := q.DeleteRatingsOfRecord(ctx, queries.DeleteRatingsOfRecordParams{TypeName: t.QualifiedName(), RecordID: id}); err != nil {
			return fmt.Errorf("records: purge ratings: %w", err)
		}
		if hasActors(t) {
			// The roles go with the record (WF3); the history keeps them.
			if err := q.DeleteActorsOfRecord(ctx, queries.DeleteActorsOfRecordParams{TypeName: t.QualifiedName(), RecordID: id}); err != nil {
				return fmt.Errorf("records: purge actors: %w", err)
			}
		}
		version := old["version"].(int64)
		if err := audit(ctx, q, t, id, version, "purge", nil, actor); err != nil {
			return err
		}
		return event(ctx, q, t, id, old, "purge", version, []string{}, nil, actor)
	})
}

// History returns the audit rows of a record, oldest first, without the changes of fields
// the session may not read (PL9; like the versions and the timeline, PL42).
func (s *Service) History(ctx context.Context, typeName string, id uuid.UUID) ([]queries.Audit, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return nil, err
	}
	if !t.HasHistory() {
		return nil, fmt.Errorf("%s has no history (trait history): %w", typeName, ErrNotFound)
	}
	// The history is as visible as the record (PL8).
	if _, err := s.Get(ctx, typeName, id); err != nil {
		return nil, err
	}
	rows, err := queries.New(s.db.Tenant(ctx)).ListAudit(ctx, queries.ListAuditParams{TypeName: typeName, RecordID: id})
	if err != nil {
		return nil, fmt.Errorf("records: history: %w", err)
	}
	readable := readableSet(ctx, t)
	return slices.DeleteFunc(rows, func(a queries.Audit) bool { return a.Field.Valid && !readable(a.Field.String) }), nil
}

// --- internals ---

type change struct {
	field    string
	old, new any
}

func audit(ctx context.Context, q *queries.Queries, t meta.Type, id uuid.UUID, version int64, action string, c *change, actor Actor) error {
	// Only a type with the trait history keeps audit rows (PL17, E94).
	if !t.HasHistory() {
		return nil
	}
	aid, err := uuid.NewV7()
	if err != nil {
		return err
	}
	p := queries.InsertAuditParams{ID: aid, TypeName: t.QualifiedName(), RecordID: id, Version: version, Action: action}
	if c != nil {
		p.Field = pgtype.Text{String: c.field, Valid: true}
		p.OldValue, _ = json.Marshal(c.old)
		p.NewValue, _ = json.Marshal(c.new)
	}
	if actor != nil {
		p.Actor = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	if tag, ok := RuleOf(ctx); ok && tag.Name != "" {
		// The rule that made the change (WF4).
		p.Rule = pgtype.Text{String: tag.Name, Valid: true}
	}
	if err := q.InsertAudit(ctx, p); err != nil {
		return fmt.Errorf("records: audit: %w", err)
	}
	return nil
}

// event writes the outbox row of a change in the same transaction (E39, PL24): the version,
// the changed fields, and org_path and created_by of the record, so that the stream can
// apply a reader's rights even after a purge (E87). The dispatcher is woken after the commit
// through AfterWrite (a NOTIFY inside the transaction would serialise every commit, LZ4).
func event(ctx context.Context, q *queries.Queries, t meta.Type, id uuid.UUID, rec Record, kind string, version int64, changed []string, extra map[string]any, actor Actor) error {
	payload := map[string]any{"version": version, "changed": changed, "org_path": rec[orgPathField], "created_by": rec["created_by"]}
	if tag, ok := RuleOf(ctx); ok {
		// A write of a rule after another write (WF4): the engine stops chains by the depth.
		payload["rule"], payload["depth"] = tag.Name, tag.Depth
	}
	for k, v := range extra {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var by uuid.NullUUID
	if actor != nil {
		by = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	if _, err := q.InsertEvent(ctx, queries.InsertEventParams{Kind: kind, TypeName: t.QualifiedName(), RecordID: id, Actor: by, Payload: raw}); err != nil {
		return fmt.Errorf("records: event: %w", err)
	}
	return nil
}

// validate checks input against the type. On create, defaults fill missing fields and
// required fields must be present; on update, only given fields are checked.
// orgPathField is the one system field a caller may set: the node of the organisation tree
// the record belongs to (PL6). It must be an existing node.
const orgPathField = "org_path"

func (s *Service) validate(ctx context.Context, t meta.Type, input map[string]any, create bool) (map[string]any, error) {
	var ps []Problem
	values := map[string]any{}
	if raw, given := input[orgPathField]; given {
		path, ok := raw.(string)
		switch {
		case !ok || !org.ValidPath(path):
			ps = append(ps, Problem{Field: orgPathField, Code: "type", Message: "must be a path of the organisation tree, like root.it"})
		default:
			exists, err := s.nodeKnown(ctx, path)
			if err != nil {
				return nil, err
			}
			if !exists {
				ps = append(ps, Problem{Field: orgPathField, Code: "unknown", Message: fmt.Sprintf("%q is not a node of the organisation tree", path)})
			} else {
				values[orgPathField] = path
			}
		}
	}
	for name := range input {
		if name == orgPathField {
			continue
		}
		if f, ok := t.Field(name); ok && f.Calculated() {
			ps = append(ps, Problem{Field: name, Code: "read_only", Message: "is calculated by the core and cannot be set"})
		} else if !ok {
			code, msg := "unknown", "is not a field of "+t.QualifiedName()
			if isSystem(name) {
				code, msg = "read_only", "is a system field and cannot be set"
			}
			ps = append(ps, Problem{Field: name, Code: code, Message: msg})
		}
	}
	for _, f := range t.Fields {
		if f.Calculated() {
			continue
		}
		v, given := input[f.Name]
		if !given {
			if create && f.Default != nil {
				v = f.Default
			} else if create && f.Required {
				ps = append(ps, Problem{Field: f.Name, Code: "required", Message: "is required"})
				continue
			} else {
				continue
			}
		}
		if v == nil && f.Required {
			ps = append(ps, Problem{Field: f.Name, Code: "required", Message: "is required"})
			continue
		}
		cv, p := coerce(f, v)
		if p != nil {
			ps = append(ps, *p)
			continue
		}
		if cv != nil && f.Type == meta.FieldReference {
			ok, err := s.refKnown(ctx, f.To, cv.(uuid.UUID))
			if err != nil {
				return nil, err
			}
			if !ok {
				ps = append(ps, Problem{Field: f.Name, Code: "reference", Message: fmt.Sprintf("no %s with id %s", f.To, cv)})
				continue
			}
		}
		values[f.Name] = cv
	}
	if len(ps) > 0 {
		return nil, &ValidationError{Problems: ps}
	}
	return values, nil
}

func (s *Service) exists(ctx context.Context, typeName string, id uuid.UUID) (bool, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var n int
	sql := fmt.Sprintf("SELECT count(*) FROM %s WHERE id = $1 AND deleted_at IS NULL", table(t))
	if c := listScope(t, ""); c != "" {
		// A value outside the list is refused even when it is an entry of another list.
		sql += " AND " + c
	}
	err = s.db.Tenant(ctx).QueryRow(ctx, sql, id).Scan(&n)
	return n > 0, err
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// read loads one record; includeTrash also finds trashed ones; lock takes FOR UPDATE.
func (s *Service) read(ctx context.Context, db querier, t meta.Type, id uuid.UUID, includeTrash, lock bool) (Record, error) {
	cols := columns(t)
	sql := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1", strings.Join(selected(t, cols), ", "), table(t))
	if c := listScope(t, ""); c != "" {
		sql += " AND " + c
	}
	if !includeTrash {
		sql += " AND deleted_at IS NULL"
	}
	if lock {
		sql += " FOR UPDATE"
	}
	rows, err := db.Query(ctx, sql, id)
	if err != nil {
		return nil, fmt.Errorf("records: read %s: %w", t.QualifiedName(), err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("%s %s: %w", t.QualifiedName(), id, ErrNotFound)
	}
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	return toRecord(t, cols, vals), nil
}

func toRecord(t meta.Type, cols []string, vals []any) Record {
	rec := Record{}
	for i, c := range cols {
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
	return rec
}

func columns(t meta.Type) []string {
	cols := append([]string{}, systemFields...)
	for _, f := range t.Fields {
		cols = append(cols, f.Name)
	}
	return append(cols, extraColumns(t)...)
}

func quoted(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = pgx.Identifier{c}.Sanitize()
	}
	return out
}

func table(t meta.Type) string { return pgx.Identifier{schema.TableName(t)}.Sanitize() }

func isSystem(name string) bool {
	for _, s := range systemFields {
		if s == name {
			return true
		}
	}
	return name == "row_hash" || name == "search"
}

func (s *Service) tx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.db.Tenant(ctx).Begin(ctx)
	if err != nil {
		return fmt.Errorf("records: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// mapDBError turns unique violations into conflicts with the field name.
func mapDBError(t meta.Type, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		field := strings.TrimSuffix(strings.TrimPrefix(pgErr.ConstraintName, schema.TableName(t)+"_"), "_key")
		return fmt.Errorf("%w: %s must be unique, this value exists", ErrConflict, field)
	}
	return fmt.Errorf("records: %s: %w", t.QualifiedName(), err)
}

func uuidOrNil(a Actor) any {
	if a == nil {
		return nil
	}
	return *a
}
