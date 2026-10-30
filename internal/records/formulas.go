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
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
)

// --- calculated fields (KF6; Hauptplan E45, E99; PL22) ---
//
// Create and Update compute the formulas of a type after validation and the rights check,
// from the record as it will be written, and store the results like given values: they show
// up in the audit and the event when they change. A formula with an empty input field gives
// an empty result; an error while computing (a division by zero, a value outside the options
// of a choice) refuses the write with 422. When a package brings a new or changed formula,
// the job records.recompute computes the field for every record of the type, without a new
// version and without audit rows.

// formulaCache holds the compiled formulas per type and set of expressions.
var formulaCache sync.Map

func formulasOf(t meta.Type) ([]meta.CompiledFormula, error) {
	var key strings.Builder
	key.WriteString(t.QualifiedName())
	for _, f := range t.Fields {
		if f.Formula != "" {
			key.WriteString("\x00" + f.Name + "=" + f.Formula)
		}
	}
	if key.Len() == len(t.QualifiedName()) {
		return nil, nil
	}
	if c, ok := formulaCache.Load(key.String()); ok {
		return c.([]meta.CompiledFormula), nil
	}
	c, err := meta.CompileFormulas(t)
	if err != nil {
		return nil, fmt.Errorf("records: %s: %w", t.QualifiedName(), err)
	}
	formulaCache.Store(key.String(), c)
	return c, nil
}

// calculate computes the formulas of the type from state (the record as it will be written,
// values in API or stored form) and puts the results into values and state.
func calculate(t meta.Type, state, values map[string]any) error {
	fs, err := formulasOf(t)
	if err != nil || len(fs) == 0 {
		return err
	}
	var ps []Problem
	for _, cf := range fs {
		name := cf.Field.Name
		result, err := evalFormula(t, cf, state)
		if err != nil {
			ps = append(ps, Problem{Field: name, Code: "formula", Message: err.Error()})
			continue
		}
		cv, p := coerce(cf.Field, result)
		if p != nil {
			ps = append(ps, Problem{Field: name, Code: "formula", Message: "the formula yields a value the field refuses: " + p.Message})
			continue
		}
		values[name] = cv
		state[name] = cv
	}
	if len(ps) > 0 {
		return &ValidationError{Problems: ps}
	}
	return nil
}

// evalFormula computes one formula; nil when a field it uses is empty.
func evalFormula(t meta.Type, cf meta.CompiledFormula, state map[string]any) (any, error) {
	vars := make(map[string]any, len(cf.Formula.Uses))
	for _, name := range cf.Formula.Uses {
		f, _ := t.Field(name)
		v, err := formulaInput(f, state[name])
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, nil
		}
		vars[name] = v
	}
	out, err := cf.Formula.Eval(vars)
	if err != nil {
		return nil, err
	}
	// Back to the API form, which coerce understands.
	if ts, ok := out.(time.Time); ok {
		if cf.Field.Type == meta.FieldDate {
			return ts.Format(dateFormat), nil
		}
		return ts.Format(time.RFC3339Nano), nil
	}
	return out, nil
}

// formulaInput turns a field value (API or stored form) into what CEL expects.
func formulaInput(f meta.Field, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case time.Time:
		return x.UTC(), nil
	case uuid.UUID:
		return x.String(), nil
	case string:
		switch f.Type {
		case meta.FieldDate:
			d, err := time.Parse(dateFormat, x)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			return d, nil
		case meta.FieldDateTime:
			d, err := time.Parse(time.RFC3339Nano, x)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			return d.UTC(), nil
		}
		if x == "" {
			return nil, nil
		}
		return x, nil
	}
	return v, nil
}

// --- the job that recomputes a type ---

// RecomputeArgs asks for the calculated fields of every record of a type to be computed again;
// with Search also its search column (PL26).
type RecomputeArgs struct {
	Type   string `json:"type"`
	Search bool   `json:"search,omitempty"`
}

// Kind names the job.
func (RecomputeArgs) Kind() string { return "records.recompute" }

type recomputeWorker struct {
	river.WorkerDefaults[RecomputeArgs]
	s      *Service
	logger *slog.Logger
}

func (w *recomputeWorker) Work(ctx context.Context, job *river.Job[RecomputeArgs]) error {
	n, failed, err := w.s.Recompute(ctx, job.Args.Type)
	if err != nil {
		return err
	}
	w.logger.Info("calculated fields recomputed", "type", job.Args.Type, "changed", n, "failed", failed)
	if job.Args.Search {
		rows, err := w.s.RecomputeSearch(ctx, job.Args.Type)
		if err != nil {
			return err
		}
		w.logger.Info("search column recomputed", "type", job.Args.Type, "records", rows)
	}
	return nil
}

// RegisterRecomputeJob adds the worker of records.recompute.
func RegisterRecomputeJob(js *jobs.Service, s *Service, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	jobs.AddWorker(js, &recomputeWorker{s: s, logger: logger})
}

// recomputeBatch is the number of records computed in one transaction.
const recomputeBatch = 500

// Recompute computes the calculated fields of every record of a type, the trash included,
// and stores the ones that changed without a new version or audit rows. It returns how many
// records changed and how many could not be computed (they keep their values).
func (s *Service) Recompute(ctx context.Context, typeName string) (changed, failed int, err error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return 0, 0, err
	}
	fs, err := formulasOf(t)
	if err != nil || len(fs) == 0 {
		return 0, 0, err
	}
	cols := columns(t)
	after := uuid.Nil
	for {
		var last uuid.UUID
		n := 0
		err := s.tx(ctx, func(tx pgx.Tx) error {
			sql := fmt.Sprintf("SELECT %s FROM %s WHERE id > $1", strings.Join(selected(t, cols), ", "), table(t))
			if c := listScope(t, ""); c != "" {
				sql += " AND " + c
			}
			sql += fmt.Sprintf(" ORDER BY id LIMIT %d FOR UPDATE", recomputeBatch)
			rows, err := tx.Query(ctx, sql, after)
			if err != nil {
				return fmt.Errorf("records: recompute %s: %w", typeName, err)
			}
			var recs []Record
			for rows.Next() {
				vals, err := rows.Values()
				if err != nil {
					rows.Close()
					return err
				}
				recs = append(recs, toRecord(t, cols, vals))
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for _, rec := range recs {
				n++
				id, _ := uuid.Parse(fmt.Sprint(rec["id"]))
				last = id
				state := make(map[string]any, len(rec))
				for k, v := range rec {
					state[k] = v
				}
				values := map[string]any{}
				if err := calculate(t, state, values); err != nil {
					failed++
					continue
				}
				var sets []string
				args := []any{id}
				for _, cf := range fs {
					name := cf.Field.Name
					if equalJSON(rec[name], toJSON(cf.Field, values[name])) {
						continue
					}
					args = append(args, values[name])
					sets = append(sets, fmt.Sprintf("%s = $%d", pgx.Identifier{name}.Sanitize(), len(args)))
				}
				if len(sets) == 0 {
					continue
				}
				if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s WHERE id = $1", table(t), strings.Join(sets, ", ")), args...); err != nil {
					return mapDBError(t, err)
				}
				if err := refreshSearch(ctx, tx, t, id); err != nil {
					return err
				}
				changed++
			}
			return nil
		})
		if err != nil {
			return changed, failed, err
		}
		if n < recomputeBatch {
			return changed, failed, nil
		}
		after = last
	}
}
