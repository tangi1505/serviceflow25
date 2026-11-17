// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// --- Day values (PL69, Hauptplan E195, Kern-Plan E73) ---
//
// A widget of a long period counts millions of rows; the day values hold per type, date field,
// day, node and group the number of records and the sums of number fields, so that it adds a
// few thousand rows instead. Which day values exist follows the widgets (your decision
// 2026-11-17: every widget that fits): a widget with the period, no condition but that its date
// is set, counting, summing or averaging, grouped by a field, a role or its date. Persons
// whose read rights are nodes only read them; whoever sees single records counts as before
// (your decision 2026-11-17). Writes mark their days through the events; a run every minute
// counts those days again, and every night all days are counted again.

// berlin is the time zone of the days, as the groups by day count them (UI13).
var berlin = mustLocation("Europe/Berlin")

func mustLocation(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// dayOf is the day in Berlin of a moment, at midnight.
func dayOf(t time.Time) time.Time {
	y, m, d := t.In(berlin).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, berlin)
}

// dayFit is what a widget asks of the day values: the date field, the group ("" for none),
// the unit of a group by the date itself, the function and its number field.
type dayFit struct {
	dateField, dim, unit, fn, measure string
}

// fitDays says whether a widget can read the day values, and what it asks of them.
func fitDays(w records.Record, src source) (dayFit, bool) {
	kind, _ := w["kind"].(string)
	if kind != KindNumber && !slices.Contains(grouped, kind) {
		return dayFit{}, false
	}
	if usePeriod, _ := w["use_period"].(bool); !usePeriod {
		return dayFit{}, false
	}
	t := src.t
	fit := dayFit{dateField: "created_at", fn: "count"}
	if df, _ := w["date_field"].(string); df != "" {
		fit.dateField = df
	}
	if fit.dateField != "created_at" {
		if f, ok := t.Field(fit.dateField); !ok || f.Type != meta.FieldDateTime {
			return dayFit{}, false
		}
	}
	// No condition, or only that the date is set: the day values know nothing else.
	if len(src.where) > 0 {
		var c map[string]any
		if json.Unmarshal(src.where, &c) != nil || len(c) != 2 || c["field"] != fit.dateField || c["null"] != false {
			return dayFit{}, false
		}
	}
	switch m, _ := w["measure"].(string); m {
	case "", "count":
	case "sum", "avg":
		mf, _ := w["measure_field"].(string)
		if f, ok := t.Field(mf); !ok || f.Type != meta.FieldNumber {
			return dayFit{}, false
		}
		fit.fn, fit.measure = m, mf
	default:
		return dayFit{}, false
	}
	if kind == KindNumber {
		return fit, true
	}
	g, _ := w["group_by"].(string)
	name, unit, byDate := strings.Cut(g, ":")
	switch {
	case byDate:
		if name != fit.dateField {
			return dayFit{}, false
		}
		fit.unit = unit
	case strings.HasPrefix(g, actorsPrefix):
		if _, ok := t.Actor(strings.TrimPrefix(g, actorsPrefix)); !ok {
			return dayFit{}, false
		}
		fit.dim = g
	case g == "state":
		fit.dim = g
	default:
		f, ok := t.Field(g)
		if !ok || strings.Contains(g, ".") || f.Type == meta.FieldDateTime || f.Type == meta.FieldDate {
			return dayFit{}, false
		}
		fit.dim = g
	}
	return fit, true
}

// daySpec is what the day values of one type and date field hold.
type daySpec struct {
	typeName, dateField string
	dims, measures      []string
}

// Days lets the dashboards keep and read day values in the databases of rt (PL69).
func (s *Service) Days(rt *db.Router) { s.days = rt }

// daySpecs reads what the widgets of all dashboards in the database of ctx ask of the day
// values; every spec has the group "" (the number of the day).
func (s *Service) daySpecs(ctx context.Context) ([]*daySpec, error) {
	byKey := map[[2]string]*daySpec{}
	var order [][2]string
	after := ""
	for {
		page, err := s.rec.List(ctx, TypeWidget, records.ListOptions{Limit: 200, After: after})
		if err != nil {
			return nil, err
		}
		for _, w := range page.Items {
			src, err := s.source(ctx, w)
			if err != nil {
				continue
			}
			fit, ok := fitDays(w, src)
			if !ok {
				continue
			}
			k := [2]string{src.typeName, fit.dateField}
			sp := byKey[k]
			if sp == nil {
				sp = &daySpec{typeName: k[0], dateField: k[1], dims: []string{""}}
				byKey[k] = sp
				order = append(order, k)
			}
			if !slices.Contains(sp.dims, fit.dim) {
				sp.dims = append(sp.dims, fit.dim)
			}
			if fit.measure != "" && !slices.Contains(sp.measures, fit.measure) {
				sp.measures = append(sp.measures, fit.measure)
			}
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	out := make([]*daySpec, 0, len(order))
	for _, k := range order {
		sp := byKey[k]
		slices.Sort(sp.dims)
		slices.Sort(sp.measures)
		out = append(out, sp)
	}
	return out, nil
}

// dayRow is one day value.
type dayRow struct {
	day          time.Time
	orgPath, dim string
	key          string
	n            int64
	sums         map[string][2]float64
}

// count counts the day values of a spec for the days [from, to) again, both at midnight in
// Berlin: one aggregate per group over the records of those days, then the old rows out and
// the new ones in, in one transaction.
func (s *Service) count(ctx context.Context, sp *daySpec, from, to time.Time) error {
	where, _ := json.Marshal(map[string]any{"and": []any{
		map[string]any{"field": sp.dateField, "gte": from.UTC().Format(time.RFC3339)},
		map[string]any{"field": sp.dateField, "lt": to.UTC().Format(time.RFC3339)},
	}})
	aggs := map[string]records.Aggregate{"n": {Count: "*"}}
	for i, m := range sp.measures {
		aggs[fmt.Sprintf("s%d", i)] = records.Aggregate{Sum: m}
		aggs[fmt.Sprintf("c%d", i)] = records.Aggregate{Count: m}
	}
	var rows []dayRow
	for _, dim := range sp.dims {
		group := []string{sp.dateField + ":day", "org_path"}
		if dim != "" {
			group = append(group, dim)
		}
		_, res, err := s.rec.Query(ctx, sp.typeName, records.Query{Where: where, Group: group, Aggregate: aggs, RowLimit: 10_000_000})
		if err != nil {
			return fmt.Errorf("dashboard: day values of %s %s: %w", sp.typeName, sp.dateField, err)
		}
		for _, r := range res.Rows {
			dayText, _ := r[sp.dateField+":day"].(string)
			day, err := time.ParseInLocation("2006-01-02", dayText, berlin)
			if err != nil {
				continue
			}
			org, _ := r["org_path"].(string)
			row := dayRow{day: day, orgPath: org, dim: dim, sums: map[string][2]float64{}}
			if dim != "" {
				raw, _ := json.Marshal(r[dim])
				row.key = string(raw)
			}
			n, _ := r["n"].(float64)
			row.n = int64(n)
			for i, m := range sp.measures {
				sum, _ := r[fmt.Sprintf("s%d", i)].(float64)
				cnt, _ := r[fmt.Sprintf("c%d", i)].(float64)
				row.sums[m] = [2]float64{sum, cnt}
			}
			rows = append(rows, row)
		}
	}
	pool := s.days.Tenant(ctx)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "DELETE FROM day_value WHERE type_name = $1 AND date_field = $2 AND day >= $3 AND day < $4",
		sp.typeName, sp.dateField, from.Format("2006-01-02"), to.Format("2006-01-02")); err != nil {
		return err
	}
	// In blocks of arrays: COPY sends ltree in its binary form, which needs a version byte pgx
	// does not write.
	const block = 10_000
	for lo := 0; lo < len(rows); lo += block {
		part := rows[lo:min(lo+block, len(rows))]
		days, orgs, dims, keys, sums := make([]string, len(part)), make([]string, len(part)), make([]string, len(part)), make([]string, len(part)), make([]string, len(part))
		ns := make([]int64, len(part))
		for i, r := range part {
			raw, _ := json.Marshal(r.sums)
			days[i], orgs[i], dims[i], keys[i], ns[i], sums[i] = r.day.Format("2006-01-02"), r.orgPath, r.dim, r.key, r.n, string(raw)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO day_value (type_name, date_field, day, org_path, dim, key, n, sums)
			SELECT $1, $2, d::date, o::ltree, m, k, n, s::jsonb FROM unnest($3::text[], $4::text[], $5::text[], $6::text[], $7::bigint[], $8::text[]) AS u(d, o, m, k, n, s)`,
			sp.typeName, sp.dateField, days, orgs, dims, keys, ns, sums); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// FillDays counts all day values of the database of ctx again (PL69: every night, your
// decision 2026-11-17), month by month so that no aggregate holds more than a month; then it
// notes for each type and date field what they hold. Day values no widget asks for any more
// go. It returns how many specs it filled.
func (s *Service) FillDays(ctx context.Context) (int, error) {
	if s.days == nil {
		return 0, nil
	}
	started := time.Now()
	specs, err := s.daySpecs(ctx)
	if err != nil {
		return 0, err
	}
	pool := s.days.Tenant(ctx)
	for _, sp := range specs {
		// The years the records have, then month by month.
		year := sp.dateField + ":year"
		_, res, err := s.rec.Query(ctx, sp.typeName, records.Query{Group: []string{year}, Aggregate: map[string]records.Aggregate{"n": {Count: "*"}}})
		if err != nil {
			return 0, err
		}
		for _, r := range res.Rows {
			text, _ := r[year].(string)
			y, err := time.ParseInLocation("2006-01-02", text, berlin)
			if err != nil {
				continue
			}
			for m := y; m.Year() == y.Year(); m = m.AddDate(0, 1, 0) {
				if err := s.count(ctx, sp, m, m.AddDate(0, 1, 0)); err != nil {
					return 0, err
				}
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM day_value WHERE type_name = $1 AND date_field = $2 AND NOT (dim = ANY($3))`,
			sp.typeName, sp.dateField, sp.dims); err != nil {
			return 0, err
		}
		if _, err := pool.Exec(ctx, `INSERT INTO day_value_state (type_name, date_field, dims, measures, filled_at) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (type_name, date_field) DO UPDATE SET dims = $3, measures = $4, filled_at = $5`,
			sp.typeName, sp.dateField, sp.dims, nonNil(sp.measures), started); err != nil {
			return 0, err
		}
		// Writes while it counted may have missed their marks: today and yesterday once more.
		today := dayOf(time.Now())
		if _, err := pool.Exec(ctx, `INSERT INTO day_value_dirty (type_name, date_field, day) VALUES ($1, $2, $3), ($1, $2, $4) ON CONFLICT DO NOTHING`,
			sp.typeName, sp.dateField, today.Format("2006-01-02"), today.AddDate(0, 0, -1).Format("2006-01-02")); err != nil {
			return 0, err
		}
	}
	// Specs no widget asks for any more.
	var keep []string
	for _, sp := range specs {
		keep = append(keep, sp.typeName+"|"+sp.dateField)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM day_value WHERE NOT (type_name || '|' || date_field = ANY($1))`, nonNil(keep)); err != nil {
		return 0, err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM day_value_state WHERE NOT (type_name || '|' || date_field = ANY($1))`, nonNil(keep)); err != nil {
		return 0, err
	}
	return len(specs), nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// CountDirtyDays counts the days writes marked again (the run every minute); it returns how
// many. A spec not filled yet has no marks.
func (s *Service) CountDirtyDays(ctx context.Context) (int, error) {
	if s.days == nil {
		return 0, nil
	}
	pool := s.days.Tenant(ctx)
	rows, err := pool.Query(ctx, `SELECT d.type_name, d.date_field, d.day, st.dims, st.measures
		FROM day_value_dirty d JOIN day_value_state st USING (type_name, date_field) ORDER BY 1, 2, 3 LIMIT 1000`)
	if err != nil {
		return 0, err
	}
	type dirty struct {
		sp  daySpec
		day time.Time
	}
	var all []dirty
	for rows.Next() {
		var d dirty
		if err := rows.Scan(&d.sp.typeName, &d.sp.dateField, &d.day, &d.sp.dims, &d.sp.measures); err != nil {
			rows.Close()
			return 0, err
		}
		d.day = time.Date(d.day.Year(), d.day.Month(), d.day.Day(), 0, 0, 0, 0, berlin)
		all = append(all, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, d := range all {
		// Out first: a write during the count marks the day again.
		if _, err := pool.Exec(ctx, "DELETE FROM day_value_dirty WHERE type_name = $1 AND date_field = $2 AND day = $3",
			d.sp.typeName, d.sp.dateField, d.day.Format("2006-01-02")); err != nil {
			return 0, err
		}
		if err := s.count(ctx, &d.sp, d.day, d.day.AddDate(0, 0, 1)); err != nil {
			_, _ = pool.Exec(ctx, "INSERT INTO day_value_dirty (type_name, date_field, day) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
				d.sp.typeName, d.sp.dateField, d.day.Format("2006-01-02"))
			return 0, err
		}
	}
	return len(all), nil
}

// ConsumerDays is the consumer of the events that marks the days of the day values.
const ConsumerDays = "day_values"

// MarkDays is the consumer of the events (PL69): a write of a type with day values marks the
// day of the write for each date field; a change of a record that may move it in the day
// values (its node, a group, a number, a date) also marks the days of its dates now. The old
// days of a moved date are not known; the count of the night sets them right.
func (s *Service) MarkDays(ctx context.Context, tx pgx.Tx, batch []records.Event) error {
	if s.days == nil || len(batch) == 0 {
		return nil
	}
	specs := map[string]map[string]dayState{}
	rows, err := tx.Query(ctx, "SELECT type_name, date_field, dims, measures FROM day_value_state")
	if err != nil {
		return err
	}
	for rows.Next() {
		var t, d string
		var st dayState
		if err := rows.Scan(&t, &d, &st.dims, &st.measures); err != nil {
			rows.Close()
			return err
		}
		if specs[t] == nil {
			specs[t] = map[string]dayState{}
		}
		specs[t][d] = st
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(specs) == 0 {
		return err
	}
	type mark struct {
		typeName, dateField string
		day                 string
	}
	marks := map[mark]bool{}
	for _, e := range batch {
		fields := specs[e.TypeName]
		if fields == nil {
			continue
		}
		switch e.Kind {
		case "create", "update", "transition", "trash", "restore":
		default:
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, e.At)
		if err != nil {
			at = time.Now()
		}
		for d := range fields {
			marks[mark{e.TypeName, d, dayOf(at).Format("2006-01-02")}] = true
		}
		if e.Kind == "create" || !moves(e, fields) {
			continue
		}
		rec, err := s.rec.Get(ctx, e.TypeName, e.RecordID)
		if err != nil {
			continue
		}
		for d := range fields {
			if v, ok := rec[d].(string); ok {
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					marks[mark{e.TypeName, d, dayOf(t).Format("2006-01-02")}] = true
				}
			}
		}
	}
	for m := range marks {
		if _, err := tx.Exec(ctx, "INSERT INTO day_value_dirty (type_name, date_field, day) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
			m.typeName, m.dateField, m.day); err != nil {
			return err
		}
	}
	return nil
}

// dayState is what the day values of a type and date field hold.
type dayState struct{ dims, measures []string }

// moves says whether a write changed something the day values of the type count by.
func moves(e records.Event, fields map[string]dayState) bool {
	var p struct {
		Changed []string `json:"changed"`
	}
	_ = json.Unmarshal(e.Payload, &p)
	if e.Kind != "update" || len(p.Changed) == 0 {
		return true
	}
	for _, c := range p.Changed {
		if c == "org_path" || c == "state" {
			return true
		}
		for d, st := range fields {
			if c == d || slices.Contains(st.dims, c) || slices.Contains(st.measures, c) {
				return true
			}
		}
	}
	return false
}

// fromDays answers a widget of a long period from the day values (PL69), when the session's
// rights are nodes only and the day values hold what it asks. A period that does not start at
// midnight starts at the midnight before it: the day values know days.
func (s *Service) fromDays(ctx context.Context, w records.Record, out Data, j *job, f Filters) (Data, bool) {
	if s.days == nil {
		return out, false
	}
	fit, ok := fitDays(w, j.src)
	if !ok || !authz.From(ctx).UnitsOnly(j.src.typeName) {
		return out, false
	}
	pool := s.days.Tenant(ctx)
	var dims, measures []string
	if err := pool.QueryRow(ctx, "SELECT dims, measures FROM day_value_state WHERE type_name = $1 AND date_field = $2",
		j.src.typeName, fit.dateField).Scan(&dims, &measures); err != nil {
		return out, false
	}
	if !slices.Contains(dims, fit.dim) || (fit.measure != "" && !slices.Contains(measures, fit.measure)) {
		return out, false
	}
	args := []any{j.src.typeName, fit.dateField, fit.dim}
	conds := []string{"type_name = $1", "date_field = $2", "dim = $3"}
	if f.From != nil {
		args = append(args, dayOf(*f.From).Format("2006-01-02"))
		conds = append(conds, fmt.Sprintf("day >= $%d", len(args)))
	}
	if f.To != nil {
		end := dayOf(*f.To)
		if !end.Equal(f.To.In(berlin)) {
			end = end.AddDate(0, 0, 1)
		}
		args = append(args, end.Format("2006-01-02"))
		conds = append(conds, fmt.Sprintf("day < $%d", len(args)))
	}
	if useUnit, _ := w["use_unit"].(bool); useUnit && f.Unit != "" {
		args = append(args, f.Unit)
		conds = append(conds, fmt.Sprintf("org_path <@ $%d::ltree", len(args)))
	}
	cond, args := authz.From(ctx).Filter(j.src.typeName, authz.ActionRead, args)
	conds = append(conds, cond)
	key := "key"
	if fit.unit != "" {
		key = fmt.Sprintf("to_char(date_trunc('%s', day)::date, 'YYYY-MM-DD')", fit.unit)
	}
	sel := "sum(n)::float8, 0::float8, 0::float8"
	if fit.measure != "" {
		args = append(args, fit.measure)
		sel = fmt.Sprintf("sum(n)::float8, coalesce(sum((sums->$%d->>0)::float8), 0), coalesce(sum((sums->$%d->>1)::float8), 0)", len(args), len(args))
	}
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT %s, %s FROM day_value WHERE %s GROUP BY 1", key, sel, strings.Join(conds, " AND ")), args...)
	if err != nil {
		return out, false
	}
	defer rows.Close()
	value := func(n, sum, cnt float64) any {
		switch fit.fn {
		case "sum":
			if cnt == 0 {
				return nil
			}
			return sum
		case "avg":
			if cnt == 0 {
				return nil
			}
			return sum / cnt
		}
		return n
	}
	type point struct {
		key   any
		value any
	}
	var points []point
	for rows.Next() {
		var k string
		var n, sum, cnt float64
		if err := rows.Scan(&k, &n, &sum, &cnt); err != nil {
			return out, false
		}
		p := point{value: value(n, sum, cnt)}
		switch {
		case fit.unit != "":
			p.key = k
		case fit.dim != "":
			_ = json.Unmarshal([]byte(k), &p.key)
		}
		points = append(points, p)
	}
	if rows.Err() != nil {
		return out, false
	}
	now := time.Now().UTC()
	out.ComputedAt = &now
	if out.Kind == KindNumber {
		out.Value = 0.0
		if len(points) == 1 && points[0].value != nil {
			out.Value = points[0].value
		}
		return out, true
	}
	// In the order of the groups as the aggregate sorts them: values first, none last.
	sort.SliceStable(points, func(a, b int) bool { return less(points[a].key, points[b].key) })
	res := &records.Rows{}
	for _, p := range points {
		res.Rows = append(res.Rows, map[string]any{j.group: p.key, "value": p.value})
	}
	out.Points = s.points(ctx, j.src.t, j.group, res)
	return out, true
}

// less orders group values: numbers by number, the rest by text, none last.
func less(a, b any) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	}
	fa, oka := a.(float64)
	fb, okb := b.(float64)
	if oka && okb {
		return fa < fb
	}
	return fmt.Sprint(a) < fmt.Sprint(b)
}

// DayValuesArgs is the run every minute that counts the marked days again.
type DayValuesArgs struct{}

// Kind names the job.
func (DayValuesArgs) Kind() string { return "dashboard.day_values" }

// FillDaysArgs is the count of all day values every night.
type FillDaysArgs struct{}

// Kind names the job.
func (FillDaysArgs) Kind() string { return "dashboard.day_values_fill" }

type dayValuesWorker struct {
	river.WorkerDefaults[DayValuesArgs]
	s *Service
}

func (w *dayValuesWorker) Work(ctx context.Context, _ *river.Job[DayValuesArgs]) error {
	n, err := w.s.CountDirtyDays(ctx)
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]int{"days": n})
}

type fillDaysWorker struct {
	river.WorkerDefaults[FillDaysArgs]
	s *Service
}

func (w *fillDaysWorker) Work(ctx context.Context, _ *river.Job[FillDaysArgs]) error {
	t0 := time.Now()
	n, err := w.s.FillDays(ctx)
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"specs": n, "seconds": int(time.Since(t0).Seconds())})
}

// Timeout lets the night's count run as long as it takes.
func (w *fillDaysWorker) Timeout(*river.Job[FillDaysArgs]) time.Duration { return -1 }

// RegisterDayValues adds the run every minute and the count every night (PL69).
func (s *Service) RegisterDayValues(js *jobs.Service) error {
	jobs.AddWorker(js, &dayValuesWorker{s: s})
	jobs.AddWorker(js, &fillDaysWorker{s: s})
	if err := js.AddPeriodic(jobs.Periodic{
		Name:     DayValuesArgs{}.Kind(),
		Label:    map[string]string{"de": "Tageswerte der Dashboards nachführen", "en": "Update the day values of the dashboards"},
		Schedule: "* * * * *",
		// Its own queue (PL70): never behind the work of a flood in "default".
		Queue: jobs.QueueDays,
		Args:  func() river.JobArgs { return DayValuesArgs{} },
	}); err != nil {
		return err
	}
	return js.AddPeriodic(jobs.Periodic{
		Name:     FillDaysArgs{}.Kind(),
		Label:    map[string]string{"de": "Tageswerte der Dashboards nachzählen", "en": "Recount the day values of the dashboards"},
		Schedule: "15 3 * * *",
		Queue:    jobs.QueueBulk,
		Args:     func() river.JobArgs { return FillDaysArgs{} },
	})
}
