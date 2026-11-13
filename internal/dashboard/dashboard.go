// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package dashboard shows dashboards (UI13, UI-Plan E39; Hauptplan KF11): a dashboard and its
// widgets are records of the core (core.dashboard, core.dashboard_widget). Whoever may read a
// dashboard sees its widgets; the numbers of a widget come from the JSON query with the rights
// of the person who looks, narrowed by the filters of the dashboard (period, unit). Whoever may
// change the dashboard saves its widgets at once.
package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// The core types.
const (
	TypeDashboard = "core.dashboard"
	TypeWidget    = "core.dashboard_widget"
	typeSearch    = "core.saved_search"
)

// Kinds of widgets.
const (
	KindNumber = "number"
	KindPie    = "pie"
	KindBar    = "bar"
	KindLine   = "line"
	KindList   = "list"
	KindText   = "text"
)

// grouped are the kinds that group their records by a field.
var grouped = []string{KindPie, KindBar, KindLine}

// systemDates are the moments every record has.
var systemDates = map[string]bool{"created_at": true, "updated_at": true, "sla_due_at": true}

// systemColumns are the columns every record has besides its fields.
var systemColumns = map[string]bool{"id": true, "version": true, "state": true, "org_path": true, "created_at": true, "updated_at": true, "sla_due_at": true}

// maxWidgets bounds the widgets of one dashboard.
const maxWidgets = 40

// Service reads and saves dashboards and computes their widgets.
type Service struct {
	rec *records.Service
}

// New returns the service.
func New(rec *records.Service) *Service { return &Service{rec: rec} }

// system works with the rights of the binary in the tenant of the request.
func system(ctx context.Context) context.Context {
	return authz.WithAccess(ctx, authz.SystemIn(authz.From(ctx).CurrentTenant()))
}

// View is a dashboard with its widgets in the order of the grid, and whether the session may
// change it.
type View struct {
	Dashboard records.Record   `json:"dashboard"`
	Widgets   []records.Record `json:"widgets"`
	Editable  bool             `json:"editable"`
}

// Get reads a dashboard the session may read, with its widgets.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (View, error) {
	d, err := s.rec.Get(ctx, TypeDashboard, id)
	if err != nil {
		return View{}, err
	}
	ws, err := s.widgets(ctx, id)
	if err != nil {
		return View{}, err
	}
	_, uerr := s.rec.Check(ctx, TypeDashboard, id, authz.ActionUpdate)
	return View{Dashboard: d, Widgets: ws, Editable: uerr == nil}, nil
}

// widgets reads the widgets of a dashboard with the rights of the binary, top left first.
func (s *Service) widgets(ctx context.Context, id uuid.UUID) ([]records.Record, error) {
	var out []records.Record
	after := ""
	for {
		page, err := s.rec.List(system(ctx), TypeWidget, records.ListOptions{Limit: 200, After: after, Filter: map[string]string{"dashboard": id.String()}})
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	num := func(r records.Record, f string) float64 { n, _ := r[f].(float64); return n }
	slices.SortStableFunc(out, func(a, b records.Record) int {
		if d := num(a, "y") - num(b, "y"); d != 0 {
			return int(d)
		}
		return int(num(a, "x") - num(b, "x"))
	})
	if out == nil {
		out = []records.Record{}
	}
	return out, nil
}

// Save stores the dashboard's fields and its widgets at once (UI13: the bar at the top saves
// everything): a widget with an id is changed, one without is created, one not named any more
// is removed. version is the dashboard's; another one is a conflict and nothing is written.
// Every widget is checked before the first write.
func (s *Service) Save(ctx context.Context, id uuid.UUID, version int64, fields map[string]any, widgets []map[string]any, actor records.Actor) (View, error) {
	d, err := s.rec.Check(ctx, TypeDashboard, id, authz.ActionUpdate)
	if err != nil {
		return View{}, err
	}
	if len(widgets) > maxWidgets {
		return View{}, &records.ValidationError{Problems: []records.Problem{{Field: "widgets", Code: "count", Message: fmt.Sprintf("a dashboard has at most %d widgets", maxWidgets)}}}
	}
	have, err := s.widgets(ctx, id)
	if err != nil {
		return View{}, err
	}
	byID := map[string]records.Record{}
	for _, w := range have {
		byID[fmt.Sprint(w["id"])] = w
	}
	sys := system(ctx)
	var ps []records.Problem
	type change struct {
		id     uuid.UUID
		values map[string]any
	}
	var changes []change
	keep := map[string]bool{}
	for i, in := range widgets {
		values := map[string]any{}
		for k, v := range in {
			if k != "id" && k != "version" && k != "dashboard" {
				values[k] = v
			}
		}
		values["dashboard"] = id.String()
		var wid uuid.UUID
		if raw, _ := in["id"].(string); raw != "" {
			if _, ok := byID[raw]; !ok {
				ps = append(ps, records.Problem{Field: fmt.Sprintf("widgets[%d].id", i), Code: "unknown", Message: "no widget of this dashboard"})
				continue
			}
			wid = uuid.MustParse(raw)
			keep[raw] = true
		}
		prefix := func(list []records.Problem) {
			for _, p := range list {
				p.Field = fmt.Sprintf("widgets[%d].%s", i, p.Field)
				ps = append(ps, p)
			}
		}
		state := values
		if wid != uuid.Nil {
			state = map[string]any{}
			for k, v := range byID[wid.String()] {
				state[k] = v
			}
			for k, v := range values {
				state[k] = v
			}
		}
		if err := s.rec.CheckValues(sys, TypeWidget, values, wid == uuid.Nil); err != nil {
			var verr *records.ValidationError
			if !errors.As(err, &verr) {
				return View{}, err
			}
			prefix(verr.Problems)
			continue
		}
		prefix(s.checkWidget(ctx, state))
		changes = append(changes, change{id: wid, values: values})
	}
	if len(ps) > 0 {
		return View{}, &records.ValidationError{Problems: ps}
	}
	// The dashboard first: its version guards the whole save.
	if fields == nil {
		fields = map[string]any{}
	}
	if _, err := s.rec.Update(ctx, TypeDashboard, id, version, fields, actor); err != nil {
		return View{}, err
	}
	for _, c := range changes {
		if c.id == uuid.Nil {
			// A new widget lives where its dashboard lives.
			c.values["org_path"] = d["org_path"]
			if _, err := s.rec.Create(sys, TypeWidget, c.values, actor); err != nil {
				return View{}, err
			}
			continue
		}
		old := byID[c.id.String()]
		v, _ := old["version"].(int64)
		if _, err := s.rec.Update(sys, TypeWidget, c.id, v, c.values, actor); err != nil {
			return View{}, err
		}
	}
	for _, w := range have {
		wid := fmt.Sprint(w["id"])
		if keep[wid] {
			continue
		}
		u := uuid.MustParse(wid)
		if err := s.rec.Trash(sys, TypeWidget, u, actor); err != nil {
			return View{}, err
		}
		if err := s.rec.Purge(sys, TypeWidget, u, actor); err != nil {
			return View{}, err
		}
	}
	return s.Get(ctx, id)
}

// Check is the RecordCheck of the widgets: what a kind needs, a source that runs, fields the
// type has. It runs the condition once with the rights of the writer.
func (s *Service) Check(ctx context.Context, t meta.Type, r map[string]any) []records.Problem {
	if t.QualifiedName() != TypeWidget {
		return nil
	}
	return s.checkWidget(ctx, r)
}

func (s *Service) checkWidget(ctx context.Context, r map[string]any) []records.Problem {
	var ps []records.Problem
	add := func(field, code, msg string) {
		ps = append(ps, records.Problem{Field: field, Code: code, Message: msg})
	}
	kind, _ := r["kind"].(string)
	if kind == KindText {
		return nil
	}
	src, err := s.source(system(ctx), r)
	if err != nil {
		var verr *records.ValidationError
		if errors.As(err, &verr) {
			return verr.Problems
		}
		add("saved_search", "invalid", err.Error())
		return ps
	}
	if len(src.where) > 0 {
		if _, _, err := s.rec.Query(ctx, src.typeName, records.Query{Where: src.where, Limit: 1}); err != nil {
			var verr *records.ValidationError
			if !errors.As(err, &verr) {
				add("query", "invalid", err.Error())
				return ps
			}
			for _, p := range verr.Problems {
				add("query", p.Code, fmt.Sprintf("%s: %s", p.Field, p.Message))
			}
		}
	}
	field := func(name string) (meta.Field, bool) { return src.t.Field(name) }
	known := func(col string) bool {
		_, ok := field(col)
		return ok || systemColumns[col]
	}
	group, _ := r["group_by"].(string)
	switch {
	case slices.Contains(grouped, kind) && group == "":
		add("group_by", "required", "a "+kind+" groups its records by a field")
	case group != "":
		name, unit, _ := strings.Cut(group, ":")
		f, ok := field(name)
		switch {
		case !ok && !systemColumns[name]:
			add("group_by", "unknown", fmt.Sprintf("%s has no field %q", src.typeName, name))
		case unit != "" && !slices.Contains([]string{"day", "week", "month", "year"}, unit):
			add("group_by", "invalid", "the unit is day, week, month or year")
		case unit != "" && !systemDates[name] && (!ok || (f.Type != meta.FieldDate && f.Type != meta.FieldDateTime)):
			add("group_by", "invalid", fmt.Sprintf("only a date groups by %s", unit))
		}
	}
	measure, _ := r["measure"].(string)
	mf, _ := r["measure_field"].(string)
	if measure != "" && measure != "count" {
		f, ok := field(mf)
		switch {
		case mf == "":
			add("measure_field", "required", measure+" needs a number field")
		case !ok || f.Type != meta.FieldNumber:
			add("measure_field", "invalid", fmt.Sprintf("%q is no number field of %s", mf, src.typeName))
		}
	}
	if df, _ := r["date_field"].(string); df != "" && !systemDates[df] {
		if f, ok := field(df); !ok || (f.Type != meta.FieldDate && f.Type != meta.FieldDateTime) {
			add("date_field", "invalid", fmt.Sprintf("%q is no date of %s", df, src.typeName))
		}
	}
	if cols, _ := r["columns"].(string); cols != "" {
		for _, c := range strings.Split(cols, ",") {
			if c = strings.TrimSpace(c); c != "" && !known(c) {
				add("columns", "unknown", fmt.Sprintf("%s has no column %q", src.typeName, c))
			}
		}
	}
	if so, _ := r["sort"].(string); so != "" && !known(strings.TrimPrefix(so, "-")) {
		add("sort", "unknown", fmt.Sprintf("%s has no column %q to sort by", src.typeName, strings.TrimPrefix(so, "-")))
	}
	return ps
}

// source is where a widget's records come from: the type, its condition and, from a saved
// search, its columns and sort.
type source struct {
	typeName string
	t        meta.Type
	where    json.RawMessage
	columns  string
	sort     string
}

// source reads the source of a widget. The saved search is read with the rights of the binary:
// its condition is no secret, the records stay those the viewer may read.
func (s *Service) source(ctx context.Context, r map[string]any) (source, error) {
	bad := func(field, code, msg string) (source, error) {
		return source{}, &records.ValidationError{Problems: []records.Problem{{Field: field, Code: code, Message: msg}}}
	}
	searchID, _ := r["saved_search"].(string)
	typeName, _ := r["target_type"].(string)
	query, _ := r["query"].(string)
	var src source
	switch {
	case searchID != "" && (typeName != "" || strings.TrimSpace(query) != ""):
		return bad("saved_search", "invalid", "a widget takes a saved search or a type with a condition, not both")
	case searchID != "":
		id, err := uuid.Parse(searchID)
		if err != nil {
			return bad("saved_search", "invalid", "is no id")
		}
		ss, err := s.rec.Get(ctx, typeSearch, id)
		if err != nil {
			return bad("saved_search", "unknown", "no saved search with this id")
		}
		src.typeName, _ = ss["target_type"].(string)
		query, _ = ss["query"].(string)
		src.columns, _ = ss["columns"].(string)
		src.sort, _ = ss["sort"].(string)
	case typeName != "":
		src.typeName = typeName
	default:
		return bad("target_type", "required", "a widget needs a saved search or a type")
	}
	t, err := s.rec.Type(ctx, src.typeName)
	if err != nil {
		return bad("target_type", "unknown", fmt.Sprintf("type %s is not installed", src.typeName))
	}
	src.t = t
	if strings.TrimSpace(query) != "" {
		var where map[string]any
		if err := json.Unmarshal([]byte(query), &where); err != nil {
			return bad("query", "type", "must be the where of a JSON query, an object like {\"field\": \"state\", \"eq\": \"new\"}")
		}
		src.where = json.RawMessage(query)
	}
	return src, nil
}

// Filters are the filters of the dashboard: a period [From, To) and a unit with all below it.
type Filters struct {
	From *time.Time `json:"from,omitempty"`
	To   *time.Time `json:"to,omitempty"`
	Unit string     `json:"unit,omitempty"`
}

// Point is one group of a pie, bar or line: its key, its label and its value.
type Point struct {
	Key   any               `json:"key"`
	Label map[string]string `json:"label,omitempty"`
	Value any               `json:"value"`
}

// Data is what a widget shows, and the type and condition behind it so that the interface can
// open the list (a click on a point adds the group's condition).
type Data struct {
	Kind    string           `json:"kind"`
	Type    string           `json:"type,omitempty"`
	Where   json.RawMessage  `json:"where,omitempty"`
	Group   string           `json:"group,omitempty"`
	Value   any              `json:"value,omitempty"`
	Points  []Point          `json:"points,omitempty"`
	Items   []records.Record `json:"items,omitempty"`
	Columns []string         `json:"columns,omitempty"`
}

// Data computes a widget of a dashboard the session may read, with the session's rights.
func (s *Service) Data(ctx context.Context, dashboardID, widgetID uuid.UUID, f Filters) (Data, error) {
	if _, err := s.rec.Get(ctx, TypeDashboard, dashboardID); err != nil {
		return Data{}, err
	}
	w, err := s.rec.Get(system(ctx), TypeWidget, widgetID)
	if err != nil {
		return Data{}, err
	}
	if fmt.Sprint(w["dashboard"]) != dashboardID.String() {
		return Data{}, fmt.Errorf("widget %s of dashboard %s: %w", widgetID, dashboardID, records.ErrNotFound)
	}
	return s.compute(ctx, w, f)
}

// compute runs the query of a widget with the rights of ctx.
func (s *Service) compute(ctx context.Context, w records.Record, f Filters) (Data, error) {
	kind, _ := w["kind"].(string)
	out := Data{Kind: kind}
	if kind == KindText {
		return out, nil
	}
	src, err := s.source(system(ctx), w)
	if err != nil {
		return out, err
	}
	out.Type = src.typeName
	var and []any
	if len(src.where) > 0 {
		and = append(and, src.where)
	}
	if usePeriod, _ := w["use_period"].(bool); usePeriod {
		df, _ := w["date_field"].(string)
		if df == "" {
			df = "created_at"
		}
		if f.From != nil {
			and = append(and, map[string]any{"field": df, "gte": f.From.UTC().Format(time.RFC3339)})
		}
		if f.To != nil {
			and = append(and, map[string]any{"field": df, "lt": f.To.UTC().Format(time.RFC3339)})
		}
	}
	if useUnit, _ := w["use_unit"].(bool); useUnit && f.Unit != "" {
		and = append(and, map[string]any{"field": "org_path", "under": f.Unit})
	}
	var where json.RawMessage
	switch len(and) {
	case 0:
	case 1:
		where, _ = json.Marshal(and[0])
	default:
		where, _ = json.Marshal(map[string]any{"and": and})
	}
	out.Where = where
	measure, _ := w["measure"].(string)
	if measure == "" {
		measure = "count"
	}
	agg := records.Aggregate{Count: "*"}
	if measure != "count" {
		mf, _ := w["measure_field"].(string)
		switch measure {
		case "sum":
			agg = records.Aggregate{Sum: mf}
		case "avg":
			agg = records.Aggregate{Avg: mf}
		case "min":
			agg = records.Aggregate{Min: mf}
		case "max":
			agg = records.Aggregate{Max: mf}
		}
	}
	switch kind {
	case KindNumber:
		_, rows, err := s.rec.Query(ctx, src.typeName, records.Query{Where: where, Aggregate: map[string]records.Aggregate{"value": agg}})
		if err != nil {
			return out, err
		}
		if rows != nil && len(rows.Rows) > 0 {
			out.Value = rows.Rows[0]["value"]
		}
		if out.Value == nil {
			out.Value = 0.0
		}
	case KindPie, KindBar, KindLine:
		group, _ := w["group_by"].(string)
		out.Group = group
		_, rows, err := s.rec.Query(ctx, src.typeName, records.Query{Where: where, Group: []string{group}, Aggregate: map[string]records.Aggregate{"value": agg}})
		if err != nil {
			return out, err
		}
		out.Points = s.points(ctx, src.t, group, rows)
	case KindList:
		cols := src.columns
		if c, _ := w["columns"].(string); c != "" {
			cols = c
		}
		for _, c := range strings.Split(cols, ",") {
			if c = strings.TrimSpace(c); c != "" {
				out.Columns = append(out.Columns, c)
			}
		}
		if len(out.Columns) == 0 {
			out.Columns = []string{src.t.TitleField}
		}
		so := src.sort
		if c, _ := w["sort"].(string); c != "" {
			so = c
		}
		size := 10
		if n, ok := w["size"].(float64); ok && n >= 1 {
			size = int(n)
		}
		q := records.Query{Where: where, Fields: out.Columns, Limit: size}
		if so != "" {
			q.Sort = []string{so}
		}
		page, _, err := s.rec.Query(ctx, src.typeName, q)
		if err != nil {
			return out, err
		}
		out.Items = page.Items
		if out.Items == nil {
			out.Items = []records.Record{}
		}
	}
	return out, nil
}

// points labels the groups: the options of a choice, the states, the titles of references.
func (s *Service) points(ctx context.Context, t meta.Type, group string, rows *records.Rows) []Point {
	out := []Point{}
	if rows == nil {
		return out
	}
	name, unit, _ := strings.Cut(group, ":")
	f, isField := t.Field(name)
	for _, row := range rows.Rows {
		key := row[group]
		p := Point{Key: key, Value: row["value"]}
		str, _ := key.(string)
		switch {
		case key == nil:
		case unit != "":
		case name == "state":
			for _, st := range t.States {
				if st.Name == str {
					p.Label = st.Label
				}
			}
		case isField && f.Type == meta.FieldChoice:
			for _, o := range f.Options {
				if o.Value == str {
					p.Label = o.Label
				}
			}
		case isField && f.Type == meta.FieldReference:
			if id, err := uuid.Parse(str); err == nil {
				if target, err := s.rec.Type(ctx, f.To); err == nil {
					if r, err := s.rec.Get(ctx, f.To, id); err == nil {
						title := fmt.Sprint(r[target.TitleField])
						p.Label = map[string]string{"de": title, "en": title}
					}
				}
			}
		}
		out = append(out, p)
	}
	return out
}
